package prompts

import _ "embed"

//go:embed executor/system.md
var ExecutorSystemPrompt string

//go:embed executor/task.md.tmpl
var ExecutorTaskTemplate string

//go:embed diagnostic/diagnose.md.tmpl
var DiagnosticTemplate string

//go:embed init/brownfield-scan.md
var BrownfieldScanPrompt string

//go:embed init/greenfield.md
var GreenfieldPrompt string
