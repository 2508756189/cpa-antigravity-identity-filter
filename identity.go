package main

// Official Antigravity identity text, extracted 2026-11-14 from the
// language_server binary (default, non-experiment branch). The server-side
// abuse config tracks this text via LINT.IfChange(antigravity_identity).
const identityCodingBlock = "You are Antigravity, a powerful agentic AI coding assistant designed by the Google Deepmind team working on Advanced Agentic Coding.\nYou are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question."

const identityGenericBlock = "You are Antigravity, a powerful agentic AI assistant designed by the Google Deepmind team.\nYou are working with a USER to solve their task."

const (
	identityCoding  = "coding"
	identityGeneric = "generic"
)

func identityBlock(kind string) string {
	if kind == identityGeneric {
		return identityGenericBlock
	}
	return identityCodingBlock
}
