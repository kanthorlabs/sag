package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kanthorlabs/sag/internal/testenv"
)

type sampler struct {
	s       *Suite
	pgName  string
	pgIP    string
	mainCtr string
	sibCtr  string
	logPath string
	seq     atomic.Uint64
	cancel  context.CancelFunc
	done    chan struct{}

	probeMu    sync.Mutex
	probeNames []string
}

func (s *Suite) StartProbe(reportDir string) func() {
	dir := filepath.Join(reportDir, fmt.Sprintf("pg%d", s.major), "logs")
	_ = os.MkdirAll(dir, 0o755)

	sm := &sampler{
		s:       s,
		logPath: filepath.Join(dir, "probe.log"),
	}
	sm.pgName, sm.pgIP, sm.mainCtr, sm.sibCtr = s.probeTargets()

	ctx, cancel := context.WithCancel(context.Background())
	sm.cancel = cancel
	sm.done = make(chan struct{})

	go sm.loop(ctx)
	return func() {
		cancel()
		<-sm.done
		sm.cleanupProbes()
	}
}

func (s *Suite) probeTargets() (pgName, pgIP, mainCtr, sibCtr string) {
	pg := s.pgs[s.major]
	if pg != nil {
		pgName = pg.Container
		pgIP = inspectIP(context.Background(), pg.Container, s.run.Network)
	}
	mainCtr = s.ContainerName(ContainerSAG)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, order := range startOrder {
		if order == ContainerSAG {
			continue
		}
		if _, ok := s.containers[order]; ok {
			sibCtr = s.ContainerName(order)
			break
		}
	}
	return pgName, pgIP, mainCtr, sibCtr
}

func inspectIP(ctx context.Context, container, network string) string {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := testenv.Podman(tctx, nil, "container", "inspect",
		"--format", "{{range $n,$v := .NetworkSettings.Networks}}{{$n}} {{$v.IPAddress}} {{end}}",
		container)
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	fields := strings.Fields(res.Stdout)
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == network {
			return fields[i+1]
		}
	}
	return ""
}

func (sm *sampler) loop(ctx context.Context) {
	defer close(sm.done)
	sm.sample(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		sm.sample(ctx)
	}
}

func (sm *sampler) sample(ctx context.Context) {
	ts := time.Now().UTC()
	statsJSON := sm.collectStats(ctx)
	mainOut := sm.probe(ctx, sm.mainCtr, "main")
	var sibOut string
	if sm.sibCtr != "" {
		sibOut = sm.probe(ctx, sm.sibCtr, "sibling")
	}
	block := formatBlock(ts, sm.pgName, sm.pgIP, sm.mainCtr, sm.sibCtr, statsJSON, mainOut, sibOut)
	f, err := os.OpenFile(sm.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(block)
	_ = f.Close()
}

func (sm *sampler) collectStats(ctx context.Context) string {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := testenv.Podman(tctx, nil, "stats", "--no-stream", "--no-reset", "--format", "json")
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return filterStatsByPrefix(res.Stdout, sm.s.run.Network)
}

func filterStatsByPrefix(raw, prefix string) string {
	var entries []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return raw
	}
	type nameOnly struct {
		Name string `json:"name"`
	}
	var filtered []json.RawMessage
	for _, e := range entries {
		var n nameOnly
		if err := json.Unmarshal(e, &n); err != nil {
			continue
		}
		if strings.HasPrefix(n.Name, prefix) {
			filtered = append(filtered, e)
		}
	}
	out, err := json.Marshal(filtered)
	if err != nil {
		return ""
	}
	return string(out)
}

func (sm *sampler) probe(ctx context.Context, ctrName, role string) string {
	n := sm.seq.Add(1)
	name := fmt.Sprintf("%s-probe-%d-%s", sm.s.run.Network, n, role[:1])

	ncTarget := sm.pgIP
	if ncTarget == "" {
		ncTarget = sm.pgName
	}
	cmd := fmt.Sprintf(
		"nslookup -type=a %s 2>&1; printf 'nslookup_exit:%%d\\n' $?; nc -z -w 2 %s 5432 2>&1; printf 'nc_exit:%%d\\n' $?",
		sm.pgName, ncTarget,
	)

	sm.probeMu.Lock()
	sm.probeNames = append(sm.probeNames, name)
	sm.probeMu.Unlock()

	start := time.Now()
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := testenv.Podman(tctx, nil,
		"run", "--rm",
		"--name", name,
		"--network", "container:"+ctrName,
		BusyboxImage,
		"sh", "-c", cmd,
	)
	dur := time.Since(start)
	if err != nil {
		return fmt.Sprintf("error: %v duration:%s\n", err, dur.Round(time.Millisecond))
	}
	out := res.Stdout
	if res.Stderr != "" {
		out += res.Stderr
	}
	return fmt.Sprintf("%sexit:%d duration:%s\n", out, res.ExitCode, dur.Round(time.Millisecond))
}

func (sm *sampler) cleanupProbes() {
	sm.probeMu.Lock()
	names := sm.probeNames
	sm.probeMu.Unlock()
	if len(names) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := append([]string{"rm", "--force", "--ignore", "--time", "0"}, names...)
	_, _ = testenv.Podman(ctx, nil, args...)
}

func formatBlock(ts time.Time, pgName, pgIP, mainCtr, sibCtr, statsJSON, mainOut, sibOut string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== %s ===\n", ts.Format(time.RFC3339Nano))
	fmt.Fprintf(&sb, "\n--- stats ---\n%s\n", statsJSON)
	fmt.Fprintf(&sb, "\n--- probe:main %s pg=%s pgip=%s ---\n%s", mainCtr, pgName, pgIP, mainOut)
	if sibCtr != "" {
		fmt.Fprintf(&sb, "\n--- probe:sibling %s ---\n%s", sibCtr, sibOut)
	}
	sb.WriteString("\n")
	return sb.String()
}
