package prompts

import _ "embed"

//go:embed executor/system.md
var ExecutorSystemPrompt string

//go:embed executor/task.md.tmpl
var ExecutorTaskTemplate string

//go:embed executor/system_parallel.md
var ParallelSystemPrompt string

//go:embed executor/reconciler.md.tmpl
var ReconcilerTemplate string

//go:embed diagnostic/diagnose.md.tmpl
var DiagnosticTemplate string

//go:embed init/brownfield-scan.md
var BrownfieldScanPrompt string

//go:embed init/greenfield.md
var GreenfieldPrompt string
