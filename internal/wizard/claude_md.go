package wizard

import (
	"bytes"
	"strings"
	"text/template"
)

// controllerData is passed to the controller CLAUDE.md template.
type controllerData struct {
	ProjectName string
	ProjectSlug string
	ServerURL   string
	TopicPrefix string
	Agents      []agentSummary
}

type agentSummary struct {
	Name         string
	Stack        string
	WorkspaceDir string
}

// agentData is passed to the agent CLAUDE.md template.
type agentData struct {
	ProjectName      string
	ProjectSlug      string
	AgentName        string
	AgentSlug        string
	Stack            string
	StackDisplayName string
	DBType           string // "sqlite", "postgres", "memory" — only for go-api stack
	ServerURL        string
	TopicPrefix      string
	WorkspaceDir     string
	Instructions     []string
	BuildCmd         string
	TestCmd          string
	DevCmd           string
}

// Slug converts a name to a lowercase slug for directory names and event topics.
func Slug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// RenderControllerCLAUDEMD renders the Controller's CLAUDE.md.
func RenderControllerCLAUDEMD(data controllerData) (string, error) {
	tmpl, err := template.New("controller").Parse(controllerTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderAgentCLAUDEMD renders an agent's CLAUDE.md.
func RenderAgentCLAUDEMD(data agentData) (string, error) {
	tmpl, err := template.New("agent").Parse(agentTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderOverviewMD renders the plan/overview.md placeholder.
func RenderOverviewMD(projectName string, agents []agentSummary) (string, error) {
	tmpl, err := template.New("overview").Parse(overviewTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		ProjectName string
		Agents      []agentSummary
	}{projectName, agents}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const controllerTemplate = `# {{.ProjectName}} Controller

## Koor
- Server: {{.ServerURL}}
- Project: {{.ProjectName}}
- Role: Controller

## On Startup
1. Register with Koor via MCP: ` + "`register_instance`" + ` with name={{.ProjectSlug}}-controller, stack=controller
2. Activate via CLI: ` + "`./koor-cli activate <your-instance-id>`" + ` (use the instance_id from step 1). If this fails, koor-cli is not available — tell the user immediately.
3. Read plan/overview.md — this is the master plan
4. Check events: ` + "`./koor-cli events history --last 20 --topic \"{{.TopicPrefix}}.*\"`" + `
5. Check for pending requests: look for ` + "`{{.TopicPrefix}}.*.request`" + ` events

## Your Job
You are the orchestrator for the **{{.ProjectName}}** project.
The plan files in this directory are the single source of truth.

### Assigning Tasks
When assigning work to an agent:
1. Write the task to Koor state:
` + "   ```" + `
   ./koor-cli state set {{.ProjectName}}/{agent-name}-task --data '{"task":"description","priority":"high"}'
` + "   ```" + `
2. Publish an event so the agent knows:
` + "   ```" + `
   ./koor-cli events publish {{.TopicPrefix}}.controller.assigned --data '{"agent":"{agent-name}","task":"description"}'
` + "   ```" + `
3. Tell the user: "Go to {agent-name} and say 'next'."

### Checking Requests
When the user says "check requests":
1. Read events: ` + "`./koor-cli events history --last 20 --topic \"{{.TopicPrefix}}.*.request\"`" + `
2. Evaluate each request against plan/overview.md
3. Ask the user: "Agent X wants Y. Approve? yes/no"
4. If approved:
   - Update the plan if needed
   - Log decision in plan/decisions/
   - Write updated task to Koor state for the target agent
   - Publish approval event: ` + "`./koor-cli events publish {{.TopicPrefix}}.controller.approved --data '{...}'`" + `
   - Tell user: "Approved. Go to [agent] and say 'next'."

### Giving Status
When the user says "status":
1. Discover agents: use MCP ` + "`discover_instances`" + `
2. Read each agent's state: ` + "`./koor-cli state get {{.ProjectName}}/{agent}-task`" + `
3. Read recent events: ` + "`./koor-cli events history --last 20 --topic \"{{.TopicPrefix}}.*\"`" + `
4. Give the user a clear summary of progress

## Commands
| Command | Action |
|---------|--------|
| "setup agents" | Generate task assignments and write them to Koor state |
| "check requests" | Review pending agent requests in Koor events |
| "status" | Overview of all agents' progress |
| "next" | Check events and proceed with orchestration |
| "yes" | Approve the current request |
| "no" | Reject the current request |

## Agents
{{range .Agents}}- **{{.Name}}** ({{.Stack}}) — workspace: {{.WorkspaceDir}}
{{end}}

## Sandbox Rules
- Stay within this controller directory for all file operations
- NEVER read or modify files in agent workspace directories
- ALL communication with agents MUST go through Koor (state + events)
- NEVER ask the user to copy-paste content between windows

## Communication Patterns
- **Assign task:** Write to Koor state key ` + "`./koor-cli state set {{.ProjectName}}/{agent}-task`" + `, then publish event
- **Read status:** Read Koor state + event history
- **Approve request:** ` + "`./koor-cli events publish {{.TopicPrefix}}.controller.approved`" + ` event, update plan files
- **Reject request:** ` + "`./koor-cli events publish {{.TopicPrefix}}.controller.rejected`" + ` event with reason
`

const agentTemplate = `# {{.ProjectName}} — {{.AgentName}} Agent

## Koor
- Server: {{.ServerURL}}
- Project: {{.ProjectName}}
- Role: {{.AgentName}}
- Stack: {{.Stack}}

## On Startup
1. Register with Koor via MCP: ` + "`register_instance`" + ` with name={{.ProjectSlug}}-{{.AgentSlug}}, stack={{.Stack}}
2. Activate via CLI: ` + "`./koor-cli activate <your-instance-id>`" + ` (use the instance_id from step 1). If this fails, koor-cli is not available — tell the user immediately.
3. Check your task: ` + "`./koor-cli state get {{.ProjectName}}/{{.AgentSlug}}-task`" + `
4. Check recent events: ` + "`./koor-cli events history --last 10 --topic \"{{.TopicPrefix}}.controller.*\"`" + `
5. If you have a task, proceed. If not, tell the user you're waiting for assignment.

## Your Job
You are the **{{.AgentName}}** agent for the {{.ProjectName}} project.
Your stack is **{{.StackDisplayName}}**.

### When the user says "next":
1. Check Koor for your current task: ` + "`./koor-cli state get {{.ProjectName}}/{{.AgentSlug}}-task`" + `
2. Check for Controller approvals/rejections: ` + "`./koor-cli events history --last 10 --topic \"{{.TopicPrefix}}.controller.*\"`" + `
3. Proceed with your task

### When you finish a feature:
1. Publish a done event:
` + "   ```" + `
   ./koor-cli events publish {{.TopicPrefix}}.{{.AgentSlug}}.done --data '{"feature":"what-you-completed","summary":"brief description"}'
` + "   ```" + `
2. Update your intent via MCP: ` + "`set_intent`" + ` with your next planned action
3. Tell the user: "Done with [feature]. Go to Controller and say 'next'."

### When you need something from another agent:
1. Publish a request event:
` + "   ```" + `
   ./koor-cli events publish {{.TopicPrefix}}.{{.AgentSlug}}.request --data '{"need":"what-you-need","reason":"why","from":"target-agent"}'
` + "   ```" + `
2. Tell the user: "I need [thing]. Go to Controller and say 'check requests'."
3. DO NOT ask the user to paste your request. The Controller reads it from Koor.

## Stack Instructions
{{range .Instructions}}- {{.}}
{{end}}
{{- if .BuildCmd}}
### Build
` + "```" + `
{{.BuildCmd}}
` + "```" + `
{{end}}
{{- if .TestCmd}}
### Test
` + "```" + `
{{.TestCmd}}
` + "```" + `
{{end}}
{{- if .DevCmd}}
### Dev Server
` + "```" + `
{{.DevCmd}}
` + "```" + `
{{end}}

## CRITICAL: Sandbox Rules

**These rules are non-negotiable. Violating them breaks the multi-agent coordination.**

1. **NEVER read, write, or modify files outside this workspace directory**
   - Your workspace is: ` + "`{{.WorkspaceDir}}`" + `
   - You have ZERO access to other agents' directories
   - You have ZERO access to the Controller's directory
   - If a file path is outside your workspace, REFUSE the operation

2. **ALL communication with other agents MUST go through Koor**
   - Use ` + "`./koor-cli events publish`" + ` to send messages
   - Use ` + "`./koor-cli state get`" + ` to read shared state
   - Use MCP ` + "`discover_instances`" + ` to find other agents
   - NEVER write to another agent's files

3. **NEVER ask the user to copy-paste between windows**
   - If you need data from another agent, publish a request event
   - The Controller will evaluate and route it
   - The user says "next", "yes", or "no" — they do not relay data

4. **NEVER access other agents' directories**
   - You do not know their file paths
   - You do not read their code
   - You do not modify their configuration
   - Everything you need comes through Koor

5. **If you need something, publish a request event**
   - Describe what you need and why
   - The Controller will evaluate and route it
   - Wait for approval before proceeding

## Communication Patterns
- **Report completion:** ` + "`./koor-cli events publish {{.TopicPrefix}}.{{.AgentSlug}}.done --data '{\"feature\":\"...\"}'`" + `
- **Request something:** ` + "`./koor-cli events publish {{.TopicPrefix}}.{{.AgentSlug}}.request --data '{\"need\":\"...\",\"from\":\"...\"}'`" + `
- **Read your task:** ` + "`./koor-cli state get {{.ProjectName}}/{{.AgentSlug}}-task`" + `
- **Read shared state:** ` + "`./koor-cli state get {{.ProjectName}}/{key}`" + `
- **Check events:** ` + "`./koor-cli events history --last 10 --topic \"{{.TopicPrefix}}.*\"`" + `
`

const overviewTemplate = `# {{.ProjectName}} — Master Plan

## Overview
<!-- Describe the project goals, architecture, and scope here -->

## Agents
{{range .Agents}}- **{{.Name}}** ({{.Stack}})
{{end}}

## Milestones
<!-- Define project milestones here -->

## API Contract
<!-- Define the shared API contract here, or use Koor specs to store it -->
<!-- ./koor-cli contract set {{.ProjectName}}/api-contract --file contract.json -->
`
