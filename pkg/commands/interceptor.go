package commands

import "context"

// InterceptSensitive executes a command immediately if it is marked Sensitive,
// returning OutcomeHandled. Non-sensitive commands and non-command inputs return
// OutcomePassthrough so they flow through the normal agent pipeline.
//
// Call this before the agent loop logs or processes input so that secret
// material (e.g. API keys passed to /secret set) never reaches the LLM context.
func (e *Executor) InterceptSensitive(ctx context.Context, req Request) ExecuteResult {
	if e == nil {
		return ExecuteResult{Outcome: OutcomePassthrough}
	}
	cmdName, ok := parseCommandName(req.Text)
	if !ok {
		return ExecuteResult{Outcome: OutcomePassthrough}
	}
	def, found := e.reg.Lookup(cmdName)
	if !found || !def.Sensitive {
		return ExecuteResult{Outcome: OutcomePassthrough}
	}
	return e.Execute(ctx, req)
}

func SensitiveDefinitions() []Definition {
	var out []Definition
	for _, d := range BuiltinDefinitions() {
		if d.Sensitive {
			out = append(out, d)
		}
	}
	return out
}
