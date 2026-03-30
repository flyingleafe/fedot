package commands

import (
	"context"
	"testing"
)

func TestInterceptSensitive_MatchesSensitiveCommand(t *testing.T) {
	called := false
	defs := []Definition{{
		Name:      "secret",
		Sensitive: true,
		Handler: func(_ context.Context, _ Request, _ *Runtime) error {
			called = true
			return nil
		},
	}}
	ex := NewExecutor(NewRegistry(defs), nil)

	res := ex.InterceptSensitive(context.Background(), Request{Text: "/secret"})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !called {
		t.Fatal("sensitive handler was not called")
	}
}

func TestInterceptSensitive_IgnoresNonSensitiveCommand(t *testing.T) {
	defs := []Definition{{
		Name:      "help",
		Sensitive: false,
		Handler: func(_ context.Context, _ Request, _ *Runtime) error {
			t.Fatal("non-sensitive handler should not be called")
			return nil
		},
	}}
	ex := NewExecutor(NewRegistry(defs), nil)

	res := ex.InterceptSensitive(context.Background(), Request{Text: "/help"})
	if res.Outcome != OutcomePassthrough {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomePassthrough)
	}
}

func TestInterceptSensitive_IgnoresNonCommand(t *testing.T) {
	defs := []Definition{{
		Name:      "secret",
		Sensitive: true,
		Handler: func(_ context.Context, _ Request, _ *Runtime) error {
			t.Fatal("handler should not be called for non-command input")
			return nil
		},
	}}
	ex := NewExecutor(NewRegistry(defs), nil)

	res := ex.InterceptSensitive(context.Background(), Request{Text: "just a message"})
	if res.Outcome != OutcomePassthrough {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomePassthrough)
	}
}

func TestInterceptSensitive_NilExecutor(t *testing.T) {
	var ex *Executor
	res := ex.InterceptSensitive(context.Background(), Request{Text: "/secret"})
	if res.Outcome != OutcomePassthrough {
		t.Fatalf("nil executor should passthrough, got=%v", res.Outcome)
	}
}

func TestInterceptSensitive_WithSubCommands(t *testing.T) {
	var handledSub string
	defs := []Definition{{
		Name:      "secret",
		Sensitive: true,
		SubCommands: []SubCommand{
			{
				Name: "set",
				Handler: func(_ context.Context, req Request, _ *Runtime) error {
					handledSub = "set"
					return nil
				},
			},
			{
				Name: "list",
				Handler: func(_ context.Context, req Request, _ *Runtime) error {
					handledSub = "list"
					return nil
				},
			},
		},
	}}
	ex := NewExecutor(NewRegistry(defs), nil)

	res := ex.InterceptSensitive(context.Background(), Request{Text: "/secret set brave mykey"})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if handledSub != "set" {
		t.Fatalf("expected 'set' subcommand, got %q", handledSub)
	}
}

func TestInterceptSensitive_TelegramBotMention(t *testing.T) {
	called := false
	defs := []Definition{{
		Name:      "secret",
		Sensitive: true,
		Handler: func(_ context.Context, _ Request, _ *Runtime) error {
			called = true
			return nil
		},
	}}
	ex := NewExecutor(NewRegistry(defs), nil)

	// Telegram sends /command@bot_name
	res := ex.InterceptSensitive(context.Background(), Request{Text: "/secret@my_bot"})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if !called {
		t.Fatal("handler should be called for /secret@bot")
	}
}
