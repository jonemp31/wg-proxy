package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"wg-proxy-manager/daemon/internal/state"
)

type process struct {
	DeviceID int
	Port     int
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
}

type Manager struct {
	mu        sync.Mutex
	processes map[int]*process
	gostBin   string
}

func NewManager(gostBin string) *Manager {
	return &Manager{
		processes: make(map[int]*process),
		gostBin:   gostBin,
	}
}

func (m *Manager) Start(dev state.Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.processes[dev.ID]; exists {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	// gost listens as SOCKS5 with auth, chains to phone's proxy via WireGuard tunnel
	listenAddr := fmt.Sprintf("socks5://%s:%s@:%d", dev.ProxyUser, dev.ProxyPass, dev.ProxyPort)
	forwardAddr := fmt.Sprintf("socks5://%s:%d", dev.WGIP, dev.PhoneProxyPort)

	cmd := exec.CommandContext(ctx, m.gostBin, "-L", listenAddr, "-F", forwardAddr)

	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start gost on port %d: %w", dev.ProxyPort, err)
	}

	slog.Info("gost started", "device_id", dev.ID, "port", dev.ProxyPort,
		"forward", forwardAddr, "pid", cmd.Process.Pid)

	proc := &process{
		DeviceID: dev.ID,
		Port:     dev.ProxyPort,
		cmd:      cmd,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	m.processes[dev.ID] = proc

	go m.supervise(proc, dev, listenAddr, forwardAddr)

	return nil
}

func (m *Manager) supervise(proc *process, dev state.Device, listenAddr, forwardAddr string) {
	defer close(proc.done)

	for {
		proc.cmd.Wait()

		if proc.cmd.ProcessState != nil && proc.cmd.ProcessState.ExitCode() == -1 {
			return
		}

		select {
		case <-proc.done:
			return
		default:
		}

		m.mu.Lock()
		current, exists := m.processes[dev.ID]
		m.mu.Unlock()
		if !exists || current != proc {
			return
		}

		slog.Warn("gost crashed, restarting", "device_id", dev.ID, "port", dev.ProxyPort)
		time.Sleep(2 * time.Second)

		ctx, cancel := context.WithCancel(context.Background())
		newCmd := exec.CommandContext(ctx, m.gostBin, "-L", listenAddr, "-F", forwardAddr)

		if err := newCmd.Start(); err != nil {
			cancel()
			slog.Error("failed to restart gost", "device_id", dev.ID, "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		m.mu.Lock()
		proc.cancel()
		proc.cmd = newCmd
		proc.cancel = cancel
		m.mu.Unlock()

		slog.Info("gost restarted", "device_id", dev.ID, "port", dev.ProxyPort, "pid", newCmd.Process.Pid)
	}
}

func (m *Manager) Stop(deviceID int) error {
	m.mu.Lock()
	proc, exists := m.processes[deviceID]
	if !exists {
		m.mu.Unlock()
		return nil
	}
	delete(m.processes, deviceID)
	m.mu.Unlock()

	proc.cancel()

	select {
	case <-proc.done:
	case <-time.After(10 * time.Second):
		if proc.cmd.Process != nil {
			proc.cmd.Process.Kill()
		}
	}

	return nil
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	procs := make([]*process, 0, len(m.processes))
	for _, p := range m.processes {
		procs = append(procs, p)
	}
	m.processes = make(map[int]*process)
	m.mu.Unlock()

	for _, p := range procs {
		p.cancel()
	}

	for _, p := range procs {
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			if p.cmd.Process != nil {
				p.cmd.Process.Kill()
			}
		}
	}
}

func (m *Manager) IsRunning(deviceID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.processes[deviceID]
	return exists
}
