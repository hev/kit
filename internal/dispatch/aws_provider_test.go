package dispatch

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func awsTestHost() Host {
	return Host{Type: "aws", Name: "test", Region: "us-east-1", From: "ami-test", Size: "t3.micro", AWS: AWSConfig{Mode: "instance", AccessKeyEnv: "TEST_AWS_KEY", SecretKeyEnv: "TEST_AWS_SECRET", Subnet: "subnet-test", SecurityGroup: "sg-test", KeyPair: "test"}}
}
func TestAWSHTTPInstanceLifecycle(t *testing.T) {
	t.Setenv("TEST_AWS_KEY", "test-key")
	t.Setenv("TEST_AWS_SECRET", "test-secret")
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		a := r.Form.Get("Action")
		calls = append(calls, a)
		if !strings.Contains(r.Header.Get("Authorization"), "test-key/") {
			t.Error("missing configured credential signature")
		}
		switch a {
		case "RunInstances":
			for _, kind := range []string{"instance", "volume"} {
				found := false
				for k, v := range r.Form {
					if strings.HasSuffix(k, "ResourceType") && v[0] == kind {
						found = true
					}
				}
				if !found {
					t.Errorf("missing %s tags", kind)
				}
			}
			if r.Form.Get("TagSpecification.1.Tag.1.Value") != "true" || r.Form.Get("MetadataOptions.HttpTokens") != "required" {
				t.Error("unsafe launch")
			}
			fmt.Fprint(w, `<RunInstancesResponse><instancesSet><item><instanceId>i-test</instanceId></item></instancesSet></RunInstancesResponse>`)
		case "DescribeInstances":
			if r.Form.Get("Filter.1.Value.1") != "true" || r.Form.Get("Filter.2.Value.1") != "test" {
				t.Error("unscoped inventory")
			}
			fmt.Fprint(w, `<DescribeInstancesResponse><reservationSet><item><instancesSet><item><instanceId>i-test</instanceId><instanceState><name>running</name></instanceState><tagSet><item><key>kit-managed</key><value>true</value></item><item><key>hev.dispatch</key><value>test</value></item></tagSet></item></instancesSet></item></reservationSet></DescribeInstancesResponse>`)
		default:
			fmt.Fprint(w, `<Response/>`)
		}
	}))
	defer server.Close()
	p, err := NewAWSProvider(awsTestHost())
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = server.URL
	m, err := p.Create(context.Background(), MachineSpec{Name: "run", Host: "test", AutoStop: time.Minute})
	if err != nil || m.ID != "i-test" {
		t.Fatalf("create: %v %v", m, err)
	}
	if err = p.Delete(context.Background(), "i-other"); err == nil {
		t.Fatal("deleted foreign instance")
	}
	if err = p.Delete(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	if calls[len(calls)-1] != "TerminateInstances" {
		t.Fatal(calls)
	}
	t.Setenv("TEST_AWS_SECRET", "")
	if p.query(context.Background(), "ec2", "DescribeInstances", url.Values{}, nil) == nil {
		t.Fatal("accepted missing named credentials")
	}
}

type fakeNitro struct{ calls []string }

func (f *fakeNitro) Call(_ context.Context, op string, in, out any) error {
	f.calls = append(f.calls, op)
	m := Machine{ID: "guest", SSH: "user@example.invalid", HostKey: "guest ssh-ed25519 test", Labels: map[string]string{"kit-managed": "true", "hev.dispatch": "test"}}
	switch op {
	case "create":
		*out.(*Machine) = m
	case "list":
		*out.(*[]Machine) = []Machine{m, {ID: "foreign", Labels: map[string]string{"hev.dispatch": "other"}}}
	}
	return nil
}
func TestAWSSandboxScopedLifecycle(t *testing.T) {
	h := awsTestHost()
	h.AWS.Mode = "sandbox"
	h.AWS.Parent = "i-parent"
	h.AWS.GuestCommand = "/usr/local/bin/nitro-guests"
	p, err := NewAWSProvider(h)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeNitro{}
	p.guests = f
	m, err := p.Create(context.Background(), MachineSpec{Host: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Delete(context.Background(), "foreign"); err == nil {
		t.Fatal("deleted foreign guest")
	}
	if err = p.Delete(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	if f.calls[len(f.calls)-1] != "delete" {
		t.Fatal(f.calls)
	}
}
func TestAWSConfigRejectsCredentialValues(t *testing.T) {
	h := awsTestHost()
	h.AWS.AccessKeyEnv = "literal-secret-value"
	if _, err := NewAWSProvider(h); err == nil {
		t.Fatal("accepted credential value")
	}
}

func TestAWSSSMAndConsoleTransport(t *testing.T) {
	t.Setenv("TEST_AWS_KEY", "test-key")
	t.Setenv("TEST_AWS_SECRET", "test-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") == "AmazonSSM.GetParameter" {
			fmt.Fprint(w, `{"Parameter":{"Value":"ami-test"}}`)
			return
		}
		r.ParseForm()
		switch r.Form.Get("Action") {
		case "RunInstances":
			if r.Form.Get("ImageId") != "ami-test" {
				t.Error("alias not resolved")
			}
			fmt.Fprint(w, `<Response><instancesSet><item><instanceId>i-test</instanceId></item></instancesSet></Response>`)
		case "DescribeInstances":
			fmt.Fprint(w, `<Response><reservationSet><item><instancesSet><item><instanceId>i-test</instanceId><ipAddress>192.0.2.1</ipAddress><tagSet><item><key>kit-managed</key><value>true</value></item><item><key>hev.dispatch</key><value>test</value></item></tagSet></item></instancesSet></item></reservationSet></Response>`)
		case "GetConsoleOutput":
			fmt.Fprintf(w, `<Response><output>%s</output></Response>`, base64.StdEncoding.EncodeToString([]byte("-----BEGIN SSH HOST KEY KEYS-----\nssh-ed25519 TEST\n-----END SSH HOST KEY KEYS-----")))
		default:
			t.Error("unexpected API action")
		}
	}))
	defer server.Close()
	h := awsTestHost()
	h.From = "/aws/service/ami-amazon-linux-latest/test"
	p, err := NewAWSProvider(h)
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = server.URL
	if _, err = p.Create(context.Background(), MachineSpec{Host: "test", AutoStop: time.Minute}); err != nil {
		t.Fatal(err)
	}
	transport, err := p.Transport(context.Background(), "i-test")
	if err != nil {
		t.Fatal(err)
	}
	if transport.HostKeyAlias != "i-test" || transport.HostKey != "i-test ssh-ed25519 TEST" {
		t.Fatal(transport)
	}
}
