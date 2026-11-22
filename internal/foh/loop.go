package foh

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// EventKind names an agent loop event.
type EventKind int

const (
	EventText       EventKind = iota // streamed assistant text
	EventToolStart                   // a tool call is about to run
	EventToolResult                  // a tool call finished
	EventSteer                       // a steer was injected
	EventModel                       // the active model changed
	EventUsage                       // token usage for one request
	EventDone                        // the user turn finished
	EventError                       // the turn failed
)

// Event is one typed loop event for the TUI or --once output.
type Event struct {
	Kind   EventKind
	Text   string
	Call   ToolCall
	Result ToolResult
	Model  string
	Usage  Usage
	Err    error
}

const (
	// DefaultMaxSteps bounds model requests in one user turn.
	DefaultMaxSteps = 50
	// MaxHistory bounds the messages kept in a thread.
	MaxHistory = 400
	steerQueue = 16
)

// SteerPrefix marks an injected steer in the thread.
const SteerPrefix = "[steer from the user, mid-turn] "

// Agent is the streaming multi-turn loop.
type Agent struct {
	Provider Provider
	Tools    *Toolset
	Session  *SessionLog
	MaxSteps int

	mu      sync.Mutex
	model   string
	system  string
	history []Message
	turn    int
	steer   chan string
}

// NewAgent returns a loop on model with the given system prompt.
func NewAgent(p Provider, tools *Toolset, model, system string) *Agent {
	return &Agent{Provider: p, Tools: tools, model: model, system: system, steer: make(chan string, steerQueue)}
}

// Model is the active model.
func (a *Agent) Model() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// SetModel hot-swaps the model for the next request; history is kept.
func (a *Agent) SetModel(id string) {
	id = strings.TrimSpace(id)
	a.mu.Lock()
	from := a.model
	if id == "" || id == from {
		a.mu.Unlock()
		return
	}
	a.model = id
	turn := a.turn
	a.mu.Unlock()
	a.Session.Write(Record{Type: "model_switch", Turn: turn, Model: id, Provider: a.ProviderFor(id), From: from, FromProvider: a.ProviderFor(from)})
}

// ProviderFor names the provider that serves model.
func (a *Agent) ProviderFor(model string) string { return providerFor(a.Provider, model) }

// System is the active system prompt.
func (a *Agent) System() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.system
}

// SetSystem replaces the system prompt for later requests (/reload).
func (a *Agent) SetSystem(s string) {
	a.mu.Lock()
	a.system = s
	a.mu.Unlock()
}

// History returns a copy of the thread.
func (a *Agent) History() []Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Message(nil), a.history...)
}

// Reset clears the thread (/clear).
func (a *Agent) Reset() {
	a.mu.Lock()
	a.history = nil
	a.mu.Unlock()
}

// Steer queues a message that lands at the next tool boundary of a running
// turn. It reports false when the queue is full.
func (a *Agent) Steer(msg string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	select {
	case a.steer <- msg:
		return true
	default:
		return false
	}
}

func (a *Agent) takeSteers() []string {
	var out []string
	for {
		select {
		case s := <-a.steer:
			out = append(out, s)
		default:
			return out
		}
	}
}

func (a *Agent) append(msgs ...Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.history = append(a.history, msgs...)
	if len(a.history) > MaxHistory {
		// Drop whole exchanges from the front so tool results stay paired.
		cut := len(a.history) - MaxHistory
		for cut < len(a.history) && a.history[cut].Role != "user" {
			cut++
		}
		a.history = append([]Message(nil), a.history[cut:]...)
	}
}

func (a *Agent) request() ChatRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	msgs := make([]Message, 0, len(a.history)+1)
	if a.system != "" {
		msgs = append(msgs, Message{Role: "system", Content: a.system})
	}
	msgs = append(msgs, a.history...)
	req := ChatRequest{Model: a.model, Messages: msgs}
	if a.Tools != nil {
		req.Tools = a.Tools.Specs()
	}
	return req
}

// syncSecrets hands every provider credential loaded so far (including
// refreshed tokens) to the session log and the tool scrubber.
func (a *Agent) syncSecrets() {
	for _, v := range providerSecrets(a.Provider) {
		a.Session.AddSecret(v)
		a.Tools.AddSecret(v)
	}
}

func (a *Agent) injectSteers(steers []string, turn int, emit func(Event)) {
	for _, s := range steers {
		a.append(Message{Role: "user", Content: SteerPrefix + s})
		a.Session.Write(Record{Type: "steer", Turn: turn, Role: "user", Content: s})
		emit(Event{Kind: EventSteer, Text: s})
	}
}

// Run executes one user turn: stream, run tools, inject steers between tool
// calls, and repeat until the model answers without tool calls.
func (a *Agent) Run(ctx context.Context, user string, emit func(Event)) error {
	if emit == nil {
		emit = func(Event) {}
	}
	a.mu.Lock()
	a.turn++
	turn := a.turn
	a.mu.Unlock()
	a.syncSecrets()
	a.append(Message{Role: "user", Content: user})
	cur := a.Model()
	a.Session.Write(Record{Type: "user", Turn: turn, Role: "user", Content: user, Model: cur, Provider: a.ProviderFor(cur)})
	steps := a.MaxSteps
	if steps <= 0 {
		steps = DefaultMaxSteps
	}
	for step := 0; step < steps; step++ {
		if err := ctx.Err(); err != nil {
			return a.fail(turn, err, emit)
		}
		req := a.request()
		res, err := a.Provider.Stream(ctx, req, func(d Delta) {
			if d.Text != "" {
				emit(Event{Kind: EventText, Text: d.Text})
			}
		})
		a.syncSecrets()
		if err != nil {
			return a.fail(turn, err, emit)
		}
		// Stamp the turn with the model we asked for; the provider's echoed
		// slug can carry a dated variant.
		model := req.Model
		provider := a.ProviderFor(model)
		a.append(Message{Role: "assistant", Content: res.Content, ToolCalls: res.ToolCalls})
		usage := res.Usage
		a.Session.Write(Record{Type: "assistant", Turn: turn, Role: "assistant", Model: model, Provider: provider, Content: res.Content, ToolCalls: res.ToolCalls, Usage: &usage})
		emit(Event{Kind: EventUsage, Usage: usage, Model: model})
		if len(res.ToolCalls) == 0 {
			if steers := a.takeSteers(); len(steers) > 0 {
				a.injectSteers(steers, turn, emit)
				continue
			}
			emit(Event{Kind: EventDone, Model: model})
			return nil
		}
		for i, call := range res.ToolCalls {
			emit(Event{Kind: EventToolStart, Call: call})
			result := a.Tools.Run(ctx, call)
			a.append(Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result.Content})
			a.Session.Write(Record{Type: "tool", Turn: turn, Role: "tool", Model: model, Provider: provider, ToolCallID: call.ID, ToolName: call.Function.Name, Content: result.Content, IsError: result.IsError})
			emit(Event{Kind: EventToolResult, Call: call, Result: result})
			steers := a.takeSteers()
			if len(steers) == 0 {
				continue
			}
			// Answer the calls the steer pre-empted so the thread stays valid.
			for _, skipped := range res.ToolCalls[i+1:] {
				msg := "skipped: the user steered before this call ran"
				a.append(Message{Role: "tool", ToolCallID: skipped.ID, Name: skipped.Function.Name, Content: msg})
				a.Session.Write(Record{Type: "tool", Turn: turn, Role: "tool", Model: model, Provider: provider, ToolCallID: skipped.ID, ToolName: skipped.Function.Name, Content: msg, IsError: true})
			}
			a.injectSteers(steers, turn, emit)
			break
		}
	}
	return a.fail(turn, fmt.Errorf("stopped after %d model requests in one turn", steps), emit)
}

func (a *Agent) fail(turn int, err error, emit func(Event)) error {
	err = scrubErr(err, providerSecrets(a.Provider)...)
	if errors.Is(err, context.Canceled) {
		err = fmt.Errorf("turn cancelled: %w", err)
	}
	cur := a.Model()
	a.Session.Write(Record{Type: "error", Turn: turn, Model: cur, Provider: a.ProviderFor(cur), Content: err.Error(), IsError: true})
	emit(Event{Kind: EventError, Err: err})
	return err
}
