package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"sidedeploy/internal/deploy"
	"sidedeploy/internal/plan"
)

func (m *model) deployTargetObj() *deploy.Target {
	list := m.targets()
	for i := range list {
		if list[i].Name == m.dTarget {
			return &list[i]
		}
	}
	if len(list) > 0 {
		return &list[0]
	}
	return nil
}

func (m *model) buildPlan() {
	m.dPlan = plan.Build(m.repo, m.cfg, m.cfgPath, m.status, m.deployTargetObj(), m.dAuto)
	m.dConfirm = false
	m.dScroll = 0
}

// openDeploy opens the upload dialog on the project's active target.
func (m *model) openDeploy() {
	tg := m.activeTarget()
	if m.repo == nil || tg == nil {
		return
	}
	m.mode = mDeploy
	m.dTarget = tg.Name
	m.dAuto = m.cfg != nil && m.cfg.AutoDelete
	m.dUnknown, m.dErr = nil, ""
	m.dLog, m.dFinished = nil, ""
	m.dDone, m.dTotal, m.dFailed = 0, 0, 0
	m.dCancelled = false
	m.buildPlan()
}

// closeDeploy closes the dialog and stops a running upload.
func (m *model) closeDeploy() {
	if m.dCancel != nil {
		m.dCancel()
		m.dCancel = nil
	}
	if m.mode == mDeploy {
		m.mode = mNone
	}
	m.dBusy, m.dConfirm = false, false
	m.dPlan = plan.Plan{}
	m.dLog, m.dUnknown, m.dFinished = nil, nil, ""
}

func (m *model) cancelDeploy() {
	if m.dCancel != nil {
		m.dCancelled = true
		m.dCancel()
	}
}

// startDeploy sends the listed jobs. A new SSH host stops the run on the
// first connection and asks to be trusted before anything goes up.
func (m *model) startDeploy() tea.Cmd {
	tg := m.deployTargetObj()
	if m.repo == nil || tg == nil || m.dBusy || len(m.dPlan.Jobs) == 0 {
		return nil
	}
	target := *tg
	jobs := append([]deploy.Job(nil), m.dPlan.Jobs...)
	workers := plan.Workers(m.cfg)
	ctx, cancel := context.WithCancel(context.Background())

	m.dBusy, m.dCancelled, m.dConfirm = true, false, false
	m.dCancel = cancel
	m.dDone, m.dTotal, m.dFailed = 0, len(jobs), 0
	m.dLog, m.dUnknown, m.dErr, m.dFinished = nil, nil, "", ""

	ch := make(chan tea.Msg, 64)
	m.dCh = ch
	dir := m.dataDir
	go func() {
		open := deploy.NewOpener(target, dir)
		deploy.Run(ctx, open, jobs, workers, func(r deploy.Result) {
			ch <- deployResultMsg{r}
		})
		ch <- deployDoneMsg{}
	}()
	return waitFor(ch)
}

func (m *model) onDeployResult(msg deployResultMsg) tea.Cmd {
	next := waitFor(m.dCh)
	if !m.dBusy {
		return next
	}
	m.dDone++
	if msg.r.Err == nil {
		return next
	}
	if u, ok := deploy.AsUnknownHost(msg.r.Err); ok {
		m.dUnknown = u
		return next
	}
	m.dFailed++
	if len(m.dLog) < 200 {
		m.dLog = append(m.dLog, msg.r.Job.Rel+": "+msg.r.Err.Error())
	}
	return next
}

func (m *model) onDeployDone() {
	if m.dCancel != nil {
		m.dCancel()
		m.dCancel = nil
	}
	m.dBusy = false
	if m.dUnknown != nil {
		// Every job failed on the same new host: ask once, not 200 times.
		m.dDone, m.dFailed = 0, 0
		m.dLog = nil
		return
	}
	name := ""
	if tg := m.deployTargetObj(); tg != nil {
		name = tg.Name
	}
	m.dFinished = plan.Outcome(name, m.dDone, m.dTotal, m.dFailed, m.dCancelled)
}

// trustHost saves the offered key and retries the upload.
func (m *model) trustHost() tea.Cmd {
	u := m.dUnknown
	if u == nil || m.dBusy {
		return nil
	}
	if err := deploy.TrustHost(m.dataDir, u); err != nil {
		m.dErr = "No se pudo guardar la huella: " + err.Error()
		return nil
	}
	m.dUnknown, m.dErr = nil, ""
	return m.startDeploy()
}
