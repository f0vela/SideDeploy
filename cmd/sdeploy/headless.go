package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/gitrepo"
	"sidedeploy/internal/plan"
	"sidedeploy/internal/projects"
)

// deployCommand sends the changes of a repo without the interface and
// returns the exit code: 0 when everything went up.
func deployCommand(dir string, args []string) int {
	var targetName, path string
	var autoDelete, yes, deleteSet, dryRun bool
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--target":
			if i+1 >= len(args) {
				return fatal("falta el nombre del destino")
			}
			i++
			targetName = args[i]
		case "--delete":
			autoDelete, deleteSet = true, true
		case "-y", "--yes":
			yes = true
		case "-n", "--dry-run":
			dryRun = true
		default:
			if strings.HasPrefix(a, "-") {
				return fatal("opción desconocida: " + a)
			}
			path = a
		}
	}
	if path == "" {
		store := projects.Load(dir)
		if path = store.Active; path == "" {
			path = "."
		}
	}
	repo, err := gitrepo.Open(expandPath(path))
	if err != nil {
		return fatal(err.Error())
	}
	cfg, cfgPath, err := deploy.Load(repo.Root)
	if err != nil {
		return fatal("config: " + err.Error())
	}
	store := projects.Load(dir)
	var targets []deploy.Target
	var proj *projects.Project
	if i := store.Index(repo.Root); i >= 0 {
		proj = &store.Projects[i]
		targets = proj.Targets
	}
	if len(targets) == 0 {
		targets = cfg.Active()
	}
	if len(targets) == 0 {
		return fatal("el proyecto no tiene destinos: créalos en SideDeploy o en pro-deployer.json")
	}
	target := &targets[0]
	if targetName == "" && proj != nil {
		if t := proj.Active(); t != nil && len(proj.Targets) > 0 {
			target = t
		}
	}
	if targetName != "" {
		target = nil
		for i := range targets {
			if targets[i].Name == targetName {
				target = &targets[i]
			}
		}
		if target == nil {
			return fatal("no hay un destino llamado " + targetName)
		}
	}
	if !deleteSet && cfg != nil {
		autoDelete = cfg.AutoDelete
	}

	st, err := repo.Status(context.Background())
	if err != nil {
		return fatal(err.Error())
	}
	p := plan.Build(repo, cfg, cfgPath, st, target, autoDelete)
	fmt.Printf("%s → %s (%s@%s:%d)\n%s\n", repo.Root, target.Name, target.User, target.Host, target.Port, p.Summary())
	for _, n := range p.Notes {
		fmt.Println("  !", n)
	}
	if len(p.Jobs) == 0 {
		return 0
	}
	for _, job := range p.Jobs {
		verb := "subir "
		if job.Delete {
			verb = "borrar"
		}
		fmt.Printf("  %s %s → %s\n", verb, job.Rel, job.Remote)
	}
	if dryRun {
		fmt.Println("(--dry-run: no se envió nada)")
		return 0
	}
	if !yes && !ask(fmt.Sprintf("¿Enviar %d archivos a %s?", len(p.Jobs), target.Name)) {
		return 1
	}
	if p.Deletes > 1 && !yes && !ask(fmt.Sprintf("Se borrarán %d archivos en el servidor. ¿Seguir?", p.Deletes)) {
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for {
		failed, unknown := runJobs(ctx, *target, dir, p.Jobs, plan.Workers(cfg))
		if unknown == nil {
			if ctx.Err() != nil {
				fmt.Println(plan.Outcome(target.Name, len(p.Jobs), len(p.Jobs), failed, true))
				return 130
			}
			fmt.Println(plan.Outcome(target.Name, len(p.Jobs), len(p.Jobs), failed, false))
			if failed > 0 {
				return 1
			}
			return 0
		}
		fmt.Printf("Host SSH nuevo: %s\nHuella %s\n", unknown.Host, unknown.Fingerprint)
		if !yes && !ask("Este host no está en known_hosts. ¿Confiar y subir?") {
			return 1
		}
		if err := deploy.TrustHost(dir, unknown); err != nil {
			return fatal("no se pudo guardar la huella: " + err.Error())
		}
	}
}

// runJobs uploads the jobs, printing each as it ends. A new host stops the
// run: it returns the host so the caller can ask to trust it.
func runJobs(ctx context.Context, t deploy.Target, dir string, jobs []deploy.Job, workers int) (failed int, unknown *deploy.UnknownHostError) {
	done := 0
	deploy.Run(ctx, deploy.NewOpener(t, dir), jobs, workers, func(r deploy.Result) {
		done++
		if r.Err != nil {
			if u, ok := deploy.AsUnknownHost(r.Err); ok {
				unknown = u
				return
			}
			failed++
			fmt.Printf("  ✗ [%d/%d] %s: %v\n", done, len(jobs), r.Rel, r.Err)
			return
		}
		verb := "↑"
		if r.Delete {
			verb = "✗ borrado"
		}
		fmt.Printf("  %s [%d/%d] %s\n", verb, done, len(jobs), r.Rel)
	})
	return failed, unknown
}

func ask(question string) bool {
	fmt.Print(question + " [s/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "s", "si", "sí", "y", "yes":
		return true
	}
	return false
}

func fatal(msg string) int {
	fmt.Fprintln(os.Stderr, "sdeploy:", msg)
	return 1
}
