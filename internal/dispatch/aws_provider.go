package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// AWSConfig contains environment names, never resolved credentials. Parent is an
// existing, tagged EC2 instance. GuestCommand is a parent-local JSON controller.
type AWSConfig struct {
	Mode            string `toml:"mode"`
	AccessKeyEnv    string `toml:"access_key_env"`
	SecretKeyEnv    string `toml:"secret_key_env"`
	SessionTokenEnv string `toml:"session_token_env"`
	Subnet          string `toml:"subnet"`
	SecurityGroup   string `toml:"security_group"`
	KeyPair         string `toml:"key_pair"`
	User            string `toml:"user"`
	Parent          string `toml:"parent"`
	GuestCommand    string `toml:"guest_command"`
}

// NitroGuests isolates the VMM/network/image implementation from EC2 lifecycle.
// Its inventory must be durable on the parent and scoped by dispatcher host.
type NitroGuests interface {
	Call(context.Context, string, any, any) error
}
type AWSProvider struct {
	host     Host
	client   *http.Client
	endpoint string
	guests   NitroGuests
}

func NewAWSProvider(h Host) (*AWSProvider, error) {
	if h.AWS.Mode == "" {
		h.AWS.Mode = "instance"
	}
	if h.AWS.User == "" {
		h.AWS.User = "ec2-user"
	}
	if h.AWS.Mode != "instance" && h.AWS.Mode != "sandbox" {
		return nil, fmt.Errorf("AWS mode must be instance or sandbox")
	}
	if h.Region == "" || h.AWS.AccessKeyEnv == "" || h.AWS.SecretKeyEnv == "" {
		return nil, fmt.Errorf("AWS requires region and credential environment names")
	}
	for _, n := range []string{h.AWS.AccessKeyEnv, h.AWS.SecretKeyEnv, h.AWS.SessionTokenEnv} {
		if n != "" && !envName.MatchString(n) {
			return nil, fmt.Errorf("invalid AWS credential environment name")
		}
	}
	if h.AWS.Mode == "instance" && (h.From == "" || h.Size == "" || h.AWS.Subnet == "" || h.AWS.SecurityGroup == "" || h.AWS.KeyPair == "") {
		return nil, fmt.Errorf("AWS instance requires image, size, subnet, security group and key pair")
	}
	if h.AWS.Mode == "sandbox" && (h.AWS.Parent == "" || h.AWS.GuestCommand == "") {
		return nil, fmt.Errorf("AWS sandbox requires parent and guest_command")
	}
	p := &AWSProvider{host: h, client: &http.Client{Timeout: 30 * time.Second}}
	p.guests = &sshNitroGuests{provider: p}
	return p, nil
}
func (p *AWSProvider) query(ctx context.Context, service, action string, v url.Values, out any) error {
	if v == nil {
		v = url.Values{}
	}
	v.Set("Action", action)
	version := "2016-11-15"
	if service == "ssm" {
		version = "2014-11-06"
	}
	v.Set("Version", version)
	key, secret := os.Getenv(p.host.AWS.AccessKeyEnv), os.Getenv(p.host.AWS.SecretKeyEnv)
	if key == "" || secret == "" {
		return fmt.Errorf("AWS credential environment is missing")
	}
	endpoint := p.endpoint
	if endpoint == "" {
		endpoint = "https://" + service + "." + p.host.Region + ".amazonaws.com/"
	}
	body := v.Encode()
	contentType := "application/x-www-form-urlencoded; charset=utf-8"
	if service == "ssm" {
		b, _ := json.Marshal(map[string]string{"Name": v.Get("Name")})
		body = string(b)
		contentType = "application/x-amz-json-1.1"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	if service == "ssm" {
		req.Header.Set("X-Amz-Target", "AmazonSSM."+action)
	}
	sum := sha256.Sum256([]byte(body))
	token := ""
	if p.host.AWS.SessionTokenEnv != "" {
		token = os.Getenv(p.host.AWS.SessionTokenEnv)
	}
	err = v4.NewSigner().SignHTTP(ctx, aws.Credentials{AccessKeyID: key, SecretAccessKey: secret, SessionToken: token}, req, hex.EncodeToString(sum[:]), service, p.host.Region, time.Now())
	if err != nil {
		return fmt.Errorf("AWS signing failed")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("AWS request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("AWS %s failed (HTTP %d; response withheld)", action, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if service == "ssm" {
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out) != nil {
			return fmt.Errorf("invalid SSM response")
		}
		return nil
	}
	if err = xml.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid AWS response")
	}
	return nil
}

type awsInstance struct {
	ID    string `xml:"instanceId"`
	IP    string `xml:"ipAddress"`
	State string `xml:"instanceState>name"`
	Tags  []struct {
		Key   string `xml:"key"`
		Value string `xml:"value"`
	} `xml:"tagSet>item"`
}

func (p *AWSProvider) machine(i awsInstance) Machine {
	labels := map[string]string{}
	for _, t := range i.Tags {
		labels[t.Key] = t.Value
	}
	address := ""
	if i.IP != "" {
		address = p.host.AWS.User + "@" + i.IP
	}
	return Machine{ID: i.ID, Name: labels["Name"], SSH: address, State: i.State, Labels: labels}
}
func (p *AWSProvider) instances(ctx context.Context, host string) ([]Machine, error) {
	var r struct {
		Reservations []struct {
			Instances []awsInstance `xml:"instancesSet>item"`
		} `xml:"reservationSet>item"`
		Next string `xml:"nextToken"`
	}
	v := url.Values{"Filter.1.Name": {"tag:kit-managed"}, "Filter.1.Value.1": {"true"}, "Filter.2.Name": {"tag:hev.dispatch"}, "Filter.2.Value.1": {host}}
	var result []Machine
	for {
		r.Next = ""
		r.Reservations = nil
		if err := p.query(ctx, "ec2", "DescribeInstances", v, &r); err != nil {
			return nil, err
		}
		for _, res := range r.Reservations {
			for _, i := range res.Instances {
				m := p.machine(i)
				if m.Labels["kit-managed"] == "true" && m.Labels["hev.dispatch"] == host && m.State != "terminated" {
					result = append(result, m)
				}
			}
		}
		if r.Next == "" {
			return result, nil
		}
		v.Set("NextToken", r.Next)
	}
}
func (p *AWSProvider) owned(ctx context.Context, id string) (Machine, error) {
	ms, err := p.instances(ctx, p.host.Name)
	if err != nil {
		return Machine{}, err
	}
	for _, m := range ms {
		if m.ID == id {
			return m, nil
		}
	}
	return Machine{}, fmt.Errorf("AWS instance is outside managed host inventory")
}
func (p *AWSProvider) List(ctx context.Context, host string) ([]Machine, error) {
	if p.host.AWS.Mode != "sandbox" {
		return p.instances(ctx, host)
	}
	var ms []Machine
	if err := p.guests.Call(ctx, "list", map[string]string{"host": host}, &ms); err != nil {
		return nil, err
	}
	var scoped []Machine
	for _, m := range ms {
		if m.Labels["hev.dispatch"] == host && m.Labels["kit-managed"] == "true" {
			scoped = append(scoped, m)
		}
	}
	return scoped, nil
}
func (p *AWSProvider) guestOwned(ctx context.Context, id string) error {
	ms, err := p.List(ctx, p.host.Name)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.ID == id {
			return nil
		}
	}
	return fmt.Errorf("Nitro guest is outside managed host inventory")
}
func (p *AWSProvider) Create(ctx context.Context, s MachineSpec) (Machine, error) {
	if s.Host != p.host.Name {
		return Machine{}, fmt.Errorf("AWS host scope mismatch")
	}
	if p.host.AWS.Mode == "sandbox" {
		var m Machine
		err := p.guests.Call(ctx, "create", s, &m)
		if err == nil && (m.ID == "" || m.SSH == "" || m.HostKey == "" || m.Labels["hev.dispatch"] != s.Host || m.Labels["kit-managed"] != "true") {
			err = fmt.Errorf("invalid Nitro guest transport or scope")
		}
		return m, err
	}
	image := s.From
	if image == "" {
		image = p.host.From
	}
	if !strings.HasPrefix(image, "ami-") {
		var r struct{ Parameter struct{ Value string } }
		if !strings.HasPrefix(image, "/aws/service/ami-amazon-linux-latest/") {
			return Machine{}, fmt.Errorf("AWS image must be an AMI ID or Amazon Linux public SSM alias")
		}
		if err := p.query(ctx, "ssm", "GetParameter", url.Values{"Name": {image}}, &r); err != nil {
			return Machine{}, err
		}
		image = r.Parameter.Value
		if !strings.HasPrefix(image, "ami-") {
			return Machine{}, fmt.Errorf("SSM alias returned invalid AMI")
		}
	}
	size := s.Size
	if size == "" {
		size = p.host.Size
	}
	v := url.Values{"ImageId": {image}, "InstanceType": {size}, "MinCount": {"1"}, "MaxCount": {"1"}, "SubnetId": {p.host.AWS.Subnet}, "SecurityGroupId.1": {p.host.AWS.SecurityGroup}, "KeyName": {p.host.AWS.KeyPair}, "MetadataOptions.HttpTokens": {"required"}, "InstanceInitiatedShutdownBehavior": {"terminate"}}
	ttl := s.AutoStop
	if ttl <= 0 {
		return Machine{}, fmt.Errorf("AWS requires a positive auto-stop duration")
	}
	minutes := int(ttl.Minutes()) + 1
	bootstrap := fmt.Sprintf("#!/bin/sh\nshutdown -h +%d\necho '-----BEGIN SSH HOST KEY KEYS-----' >/dev/console\ncat /etc/ssh/ssh_host_ed25519_key.pub >/dev/console\necho '-----END SSH HOST KEY KEYS-----' >/dev/console\n", minutes)
	v.Set("UserData", base64.StdEncoding.EncodeToString([]byte(bootstrap)))
	// Tag volumes as well so IAM can constrain launch and cleanup.
	for n, kind := range []string{"instance", "volume"} {
		prefix := fmt.Sprintf("TagSpecification.%d.", n+1)
		v.Set(prefix+"ResourceType", kind)
		for j, t := range [][2]string{{"kit-managed", "true"}, {"hev.dispatch", s.Host}, {"Name", s.Name}} {
			v.Set(fmt.Sprintf("%sTag.%d.Key", prefix, j+1), t[0])
			v.Set(fmt.Sprintf("%sTag.%d.Value", prefix, j+1), t[1])
		}
	}
	var r struct {
		Instances []awsInstance `xml:"instancesSet>item"`
	}
	if err := p.query(ctx, "ec2", "RunInstances", v, &r); err != nil {
		return Machine{}, err
	}
	if len(r.Instances) != 1 || r.Instances[0].ID == "" {
		return Machine{}, fmt.Errorf("AWS launch returned no instance")
	}
	return p.machine(r.Instances[0]), nil
}
func (p *AWSProvider) Transport(ctx context.Context, id string) (MachineTransport, error) {
	if p.host.AWS.Mode == "sandbox" {
		ms, err := p.List(ctx, p.host.Name)
		if err != nil {
			return MachineTransport{}, err
		}
		for _, m := range ms {
			if m.ID == id {
				return MachineTransport{Kind: TransportSSH, SSH: m.SSH, HostKey: m.HostKey}, nil
			}
		}
		return MachineTransport{}, fmt.Errorf("Nitro guest not found")
	}
	m, err := p.owned(ctx, id)
	if err != nil {
		return MachineTransport{}, err
	}
	var r struct {
		Output string `xml:"output"`
	}
	if err = p.query(ctx, "ec2", "GetConsoleOutput", url.Values{"InstanceId": {id}, "Latest": {"true"}}, &r); err != nil {
		return MachineTransport{}, err
	}
	data, err := base64.StdEncoding.DecodeString(r.Output)
	if err != nil {
		return MachineTransport{}, fmt.Errorf("invalid EC2 console encoding")
	}
	var key string
	inside := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "-----BEGIN SSH HOST KEY KEYS-----" {
			inside = true
			continue
		}
		if line == "-----END SSH HOST KEY KEYS-----" {
			inside = false
		}
		if inside && strings.HasPrefix(line, "ssh-ed25519 ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				key = fields[0] + " " + fields[1]
			}
		}
	}
	if key == "" || m.SSH == "" {
		return MachineTransport{}, fmt.Errorf("EC2 authenticated SSH host key or address not ready")
	}
	return MachineTransport{Kind: TransportSSH, SSH: m.SSH, HostKey: id + " " + key, HostKeyAlias: id}, nil
}
func (p *AWSProvider) Exec(ctx context.Context, id, cmd string) (string, error) {
	t, err := p.Transport(ctx, id)
	if err != nil {
		return "", err
	}
	return sshMachineExec(ctx, t, cmd)
}
func (p *AWSProvider) Copy(ctx context.Context, id, path string) ([]byte, error) {
	s, err := p.Exec(ctx, id, "cat -- "+Quote(path))
	return []byte(s), err
}
func (p *AWSProvider) WaitReady(ctx context.Context, id string) error {
	for {
		if _, err := p.Exec(ctx, id, "true"); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func (p *AWSProvider) mutate(ctx context.Context, id, op string) error {
	if p.host.AWS.Mode == "sandbox" {
		if err := p.guestOwned(ctx, id); err != nil {
			return err
		}
		return p.guests.Call(ctx, op, map[string]string{"id": id, "host": p.host.Name}, nil)
	}
	if _, err := p.owned(ctx, id); err != nil {
		return err
	}
	action := map[string]string{"stop": "StopInstances", "resume": "StartInstances", "delete": "TerminateInstances"}[op]
	return p.query(ctx, "ec2", action, url.Values{"InstanceId.1": {id}}, nil)
}
func (p *AWSProvider) Stop(ctx context.Context, id string) error { return p.mutate(ctx, id, "stop") }
func (p *AWSProvider) Delete(ctx context.Context, id string) error {
	return p.mutate(ctx, id, "delete")
}
func (p *AWSProvider) Resume(ctx context.Context, id string, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("AWS resume requires a positive lifetime")
	}
	if err := p.mutate(ctx, id, "resume"); err != nil {
		return err
	}
	if p.host.AWS.Mode == "sandbox" {
		return nil
	}
	if err := p.WaitReady(ctx, id); err != nil {
		return err
	}
	_, err := p.Exec(ctx, id, fmt.Sprintf("sudo -n shutdown -h +%d", int(ttl.Minutes())+1))
	return err
}

type sshNitroGuests struct{ provider *AWSProvider }

func (g *sshNitroGuests) Call(ctx context.Context, op string, input, output any) error {
	p := *g.provider
	p.host.AWS.Mode = "instance"
	t, err := p.Transport(ctx, p.host.AWS.Parent)
	if err != nil {
		return err
	}
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	out, err := sshMachineExec(ctx, t, "printf %s "+Quote(base64.StdEncoding.EncodeToString(b))+" | base64 -d | "+Quote(p.host.AWS.GuestCommand)+" "+Quote(op))
	if err != nil {
		return err
	}
	if output != nil {
		if json.Unmarshal([]byte(out), output) != nil {
			return fmt.Errorf("invalid Nitro controller JSON")
		}
	}
	return nil
}

var _ CloudProvider = (*AWSProvider)(nil)
