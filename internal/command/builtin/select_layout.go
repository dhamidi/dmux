package builtin

import (
	"github.com/dhamidi/dmux/internal/command"
)

func init() {
	command.Register(command.Spec{
		Name:  "select-layout",
		Alias: []string{"selectl"},
		Args: command.ArgSpec{
			Flags:   []string{"E", "n", "o", "p"},
			MaxArgs: 1,
		},
		Target: command.TargetSpec{
			Kind:     command.TargetWindow,
			Optional: true,
		},
		Run: runSelectLayout,
	})
}

func runSelectLayout(ctx *command.Ctx) command.Result {
	if ctx.Mutator == nil {
		return command.Errorf("select-layout: no mutator available")
	}
	win := ctx.Target.Window
	sess := ctx.Target.Session

	var spec string
	switch {
	case ctx.Args.Flag("n"):
		spec = "next"
	case ctx.Args.Flag("p"):
		spec = "prev"
	case ctx.Args.Flag("o"):
		spec = "undo"
	case ctx.Args.Flag("E"):
		spec = "even"
	case len(ctx.Args.Positional) > 0:
		spec = ctx.Args.Positional[0]
	default:
		// No layout specified: re-apply the current preset (or even-horizontal).
		spec = win.CurrentPreset
		if spec == "" {
			spec = "even-horizontal"
		}
	}

	if err := ctx.Mutator.ApplyLayout(sess.ID, win.ID, spec); err != nil {
		return command.Errorf("select-layout: %v", err)
	}
	return command.OK()
}
