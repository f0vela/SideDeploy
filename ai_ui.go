package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"sidedeploy/internal/aicommits"
)

// aiTool is the command line the user chose, Claude when they chose none.
func (a *app) aiTool() string {
	for _, t := range aicommits.Tools {
		if a.store.AITool == t {
			return t
		}
	}
	return aicommits.Claude
}

// generateMessage asks the AI for a message describing the staged changes,
// or every change when nothing is staged. It never commits on its own: the
// message lands in the box for the user to edit.
func (a *app) generateMessage() {
	repo := a.repo
	if repo == nil || a.aiBusy {
		return
	}
	tool := a.aiTool()
	a.aiBusy = true
	a.err = ""
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	a.aiCancel = cancel

	go func() {
		diff, err := repo.ChangeText(ctx)
		history := ""
		if err == nil {
			history = strings.Join(repo.RecentMessages(ctx, 10), "\n")
		}
		var msg string
		if err == nil {
			msg, err = aicommits.Generate(ctx, tool, diff, history)
		}
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		cancel()
		a.win.Update(func() {
			a.aiBusy = false
			a.aiCancel = nil
			if repo != a.repo {
				return
			}
			switch {
			case errors.Is(err, context.Canceled):
				// The user canceled: keep what they had written.
			case errors.Is(err, context.DeadlineExceeded):
				a.showError("La IA tardó demasiado",
					"El límite son 2 minutos. Prueba de nuevo o con menos cambios en el stage.",
					"El comando no terminó a tiempo.")
			case err != nil:
				a.showError("La IA no generó el mensaje",
					"Revisa que la herramienta esté instalada y con la sesión iniciada.",
					err.Error())
			default:
				a.commitMsg = msg
			}
		})
	}()
}

// cancelAI stops the command that is writing a message.
func (a *app) cancelAI() {
	if a.aiCancel != nil {
		a.aiCancel()
	}
}

func (a *app) showError(title, hint, text string) {
	a.errorTitle = title
	a.errorHint = hint
	a.errorText = text
	a.errorOpen = true
}
