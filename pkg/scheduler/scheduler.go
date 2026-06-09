package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Task defines a single scheduled task.
type Task struct {
	// Name uniquely identifies the task (used for logging and management).
	Name string
	// CronExpr is a 5-field cron expression (minute hour day month weekday).
	// Supports "*", "*/N", comma-separated values, and ranges.
	// Example: "*/5 * * * *" (every 5 minutes), "0 9 * * 1-5" (9 AM weekdays).
	CronExpr string
	// AgentID is the ID of the agent to execute this task.
	AgentID string
	// Prompt is the task prompt sent to the agent.
	Prompt string
	// Timeout controls the maximum execution time per run (default: 5 minutes).
	Timeout time.Duration
	// Enabled can be toggled at runtime to pause/resume a task.
	Enabled bool
}

// Runner is the interface for executing scheduled tasks.
// This allows the scheduler to work with any agent runtime.
type Runner interface {
	ExecuteTask(ctx context.Context, agentID, prompt string) (string, error)
}

// Config holds the scheduler configuration.
type Config struct {
	// Tasks is the list of scheduled tasks.
	Tasks []Task
	// Runner executes scheduled tasks. If nil, tasks will log a warning.
	Runner Runner
	// Logger is used for scheduler logging.
	Logger *zap.Logger
	// Location sets the timezone for cron expressions (default: UTC).
	Location *time.Location
}

// Scheduler is a cron-style task scheduler that implements service.Service.
type Scheduler struct {
	tasks    []taskEntry
	runner   Runner
	logger   *zap.Logger
	location *time.Location
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
}

type taskEntry struct {
	Task
	nextRun time.Time
	cron    *cronSchedule
}

// New creates a new Scheduler.
func New(cfg Config) *Scheduler {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	loc := cfg.Location
	if loc == nil {
		loc = time.UTC
	}
	s := &Scheduler{
		runner:   cfg.Runner,
		logger:   cfg.Logger.With(zap.String("component", "scheduler")),
		location: loc,
	}
	for _, t := range cfg.Tasks {
		if t.Timeout == 0 {
			t.Timeout = 5 * time.Minute
		}
		if !t.Enabled {
			t.Enabled = true
		}
		sched, err := parseCron(t.CronExpr)
		if err != nil {
			s.logger.Warn("invalid cron expression; task disabled",
				zap.String("task", t.Name), zap.String("cron", t.CronExpr), zap.Error(err))
			continue
		}
		now := time.Now().In(loc)
		s.tasks = append(s.tasks, taskEntry{
			Task:    t,
			nextRun: sched.Next(now),
			cron:    sched,
		})
	}
	return s
}

// Name returns the service name.
func (s *Scheduler) Name() string { return "scheduler" }

// Start begins scheduling tasks. Implements service.Service.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.wg.Add(1)
	go s.loop()
	s.logger.Info("scheduler started", zap.Int("tasks", len(s.tasks)))
	return nil
}

// Stop gracefully shuts down the scheduler. Implements service.Service.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	s.logger.Info("scheduler stopped")
	return nil
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.runDueTasks(now.In(s.location))
		}
	}
}

func (s *Scheduler) runDueTasks(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tasks {
		t := &s.tasks[i]
		if !t.Enabled {
			continue
		}
		if now.Before(t.nextRun) {
			continue
		}
		t.nextRun = t.cron.Next(now)
		s.logger.Info("running scheduled task",
			zap.String("task", t.Name), zap.String("agent_id", t.AgentID))
		go s.executeTask(*t)
	}
}

func (s *Scheduler) executeTask(t taskEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), t.Timeout)
	defer cancel()
	if s.runner == nil {
		s.logger.Warn("no runner configured; skipping task",
			zap.String("task", t.Name))
		return
	}
	result, err := s.runner.ExecuteTask(ctx, t.AgentID, t.Prompt)
	if err != nil {
		s.logger.Error("scheduled task failed",
			zap.String("task", t.Name), zap.Error(err))
		return
	}
	s.logger.Info("scheduled task completed",
		zap.String("task", t.Name),
		zap.String("result", truncate(result, 200)))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// cronSchedule holds a pre-parsed cron schedule.
type cronSchedule struct {
	minutes  fieldMatcher
	hours    fieldMatcher
	days     fieldMatcher
	months   fieldMatcher
	weekdays fieldMatcher
}

type fieldMatcher struct {
	values map[int]bool
	all    bool
}

func parseCron(expr string) (*cronSchedule, error) {
	fields := splitFields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}
	s := &cronSchedule{}
	var err error
	s.minutes, err = parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minutes: %w", err)
	}
	s.hours, err = parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hours: %w", err)
	}
	s.days, err = parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("days: %w", err)
	}
	s.months, err = parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("months: %w", err)
	}
	s.weekdays, err = parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("weekdays: %w", err)
	}
	return s, nil
}

func splitFields(expr string) []string {
	var fields []string
	current := ""
	for _, ch := range expr {
		if ch == ' ' || ch == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

func parseField(field string, min, max int) (fieldMatcher, error) {
	if field == "*" {
		return fieldMatcher{all: true}, nil
	}
	values := make(map[int]bool)
	parts := splitComma(field)
	for _, part := range parts {
		if stepIdx := indexOf(part, "/"); stepIdx >= 0 {
			rangePart := part[:stepIdx]
			step, err := parseInt(part[stepIdx+1:])
			if err != nil || step < 1 {
				return fieldMatcher{}, fmt.Errorf("invalid step in %q", part)
			}
			rangeMin, rangeMax := min, max
			if rangePart != "*" {
				if dashIdx := indexOf(rangePart, "-"); dashIdx >= 0 {
					rangeMin, _ = parseInt(rangePart[:dashIdx])
					rangeMax, _ = parseInt(rangePart[dashIdx+1:])
				} else {
					rangeMin, _ = parseInt(rangePart)
					rangeMax = max
				}
			}
			for v := rangeMin; v <= rangeMax; v += step {
				if v >= min && v <= max {
					values[v] = true
				}
			}
		} else if dashIdx := indexOf(part, "-"); dashIdx >= 0 {
			start, err1 := parseInt(part[:dashIdx])
			end, err2 := parseInt(part[dashIdx+1:])
			if err1 != nil || err2 != nil {
				return fieldMatcher{}, fmt.Errorf("invalid range in %q", part)
			}
			for v := start; v <= end; v++ {
				values[v] = true
			}
		} else {
			v, err := parseInt(part)
			if err != nil {
				return fieldMatcher{}, fmt.Errorf("invalid value %q", part)
			}
			values[v] = true
		}
	}
	return fieldMatcher{values: values}, nil
}

func splitComma(s string) []string {
	var parts []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	parts = append(parts, current)
	return parts
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func parseInt(s string) (int, error) {
	var n int
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a number: %q", s)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

// Next returns the next time the schedule fires after `after`.
func (s *cronSchedule) Next(after time.Time) time.Time {
	// Start from the next minute, incrementing until we find a match.
	t := after.Truncate(time.Minute).Add(time.Minute)
	// Limit search to avoid infinite loop.
	for i := 0; i < 525600; i++ { // search up to 1 year
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return after.Add(24 * time.Hour)
}

func (s *cronSchedule) matches(t time.Time) bool {
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	minute := t.Minute()
	weekday := int(t.Weekday())
	return s.months.match(month) && s.days.match(day) &&
		s.hours.match(hour) && s.minutes.match(minute) &&
		s.weekdays.match(weekday)
}

func (f fieldMatcher) match(v int) bool {
	if f.all {
		return true
	}
	return f.values[v]
}