package cmd

import (
	"errors"
	"os/exec"
	"sync"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type ExecCmdBuilder interface {
	Build(runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error)
}

type Manager struct {
	builder ExecCmdBuilder

	current *CurrentCmd
	mutex   sync.Mutex
}

func NewManager(builder ExecCmdBuilder, currentCmd *CurrentCmd) *Manager {
	return &Manager{
		builder: builder,
		current: currentCmd,
	}
}

var _ domain.Cmd = (*Manager)(nil)

func (c *Manager) Run(runDir domain.Path, event *domain.Event) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.current.Terminate()

	cmds, err := c.builder.Build(runDir, event)
	if err != nil {
		return err
	}

	go func() {
		for _, cmd := range cmds {
			if err := c.current.Start(cmd); err != nil {
				helper.GlobalLogger.Debug("Manager/Run/Start: " + err.Error())
			}

			stop, err := c.current.Wait()
			if err != nil {
				helper.GlobalLogger.Debug("Manager/Run/Wait: " + err.Error())
			}
			c.current.Clear()
			if stop {
				break
			}
		}
	}()

	return nil
}

type CurrentCmd struct {
	cmd   *exec.Cmd
	mutex sync.Mutex
}

func (c *CurrentCmd) Terminate() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		if err := terminateProcess(c.cmd); err != nil {
			helper.GlobalLogger.Debug("CurrentCmd/Terminate: " + err.Error())
		}
	}
	c.cmd = nil
}

func (c *CurrentCmd) Start(cmd *exec.Cmd) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd

	return nil
}

func (c *CurrentCmd) Wait() (stop bool, err error) {
	if c.cmd == nil {
		return false, nil
	}

	if err := c.cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return true, nil
		} else {
			return false, err
		}
	}

	return false, nil
}

func (c *CurrentCmd) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cmd = nil
}
