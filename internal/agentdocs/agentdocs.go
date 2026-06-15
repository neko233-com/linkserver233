package agentdocs

import _ "embed"

// AgentGuide is the canonical agent/LLM usage guide, served by the CLI `agent`
// command and the HTTP `/agent` endpoint.
//
//go:embed agent.md
var AgentGuide string

// LLMSText is the machine-readable index served at `/llms.txt`.
//
//go:embed llms.txt
var LLMSText string
