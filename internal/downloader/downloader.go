package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"darkorbit-resource-downloader/internal/model"
)

type Result struct {
	Item model.DownloadItem
	Err  error
}

type Stats struct {
	EffectiveConcurrency int
	MaxConcurrency       int
	MinConcurrency       int
	InFlight             int
	SuccessStreak        int
	AutoTuneEnabled      bool
}

type Client struct {
	httpClient *http.Client
	pacer      *adaptivePacer
	limiter    *adaptiveConcurrency
}

type HTTPStatusError struct {
	StatusCode int
	URL        string
	RetryAfter time.Duration
}

type Config struct {
	Timeout             time.Duration
	RequestInterval     time.Duration
	MaxCooldown         time.Duration
	MaxConcurrency      int
	MinConcurrency      int
	AutoTuneConcurrency bool
}

type adaptivePacer struct {
	minInterval time.Duration
	maxCooldown time.Duration

	mu          sync.Mutex
	nextRequest time.Time
	cooldownEnd time.Time
	penalty     int
}

type adaptiveConcurrency struct {
	enabled bool
	max     int
	min     int

	mu            sync.Mutex
	inFlight      int
	current       int
	successStreak int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status %d for %s", e.StatusCode, e.URL)
}

func New() *Client {
	return NewWithConfig(Config{
		Timeout:             90 * time.Second,
		RequestInterval:     250 * time.Millisecond,
		MaxCooldown:         20 * time.Second,
		MaxConcurrency:      8,
		MinConcurrency:      1,
		AutoTuneConcurrency: true,
	})
}

func NewWithConfig(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.RequestInterval < 0 {
		cfg.RequestInterval = 0
	}
	if cfg.MaxCooldown <= 0 {
		cfg.MaxCooldown = 20 * time.Second
	}
	if cfg.MaxConcurrency < 1 {
		cfg.MaxConcurrency = 1
	}
	if cfg.MinConcurrency < 1 {
		cfg.MinConcurrency = 1
	}
	if cfg.MinConcurrency > cfg.MaxConcurrency {
		cfg.MinConcurrency = cfg.MaxConcurrency
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		pacer:   newAdaptivePacer(cfg.RequestInterval, cfg.MaxCooldown),
		limiter: newAdaptiveConcurrency(cfg.MaxConcurrency, cfg.MinConcurrency, cfg.AutoTuneConcurrency),
	}
}

func (c *Client) FetchToFile(ctx context.Context, url, destination string) error {
	if err := c.limiter.Acquire(ctx); err != nil {
		return err
	}
	defer c.limiter.Release()

	return c.fetchToFileWithRetry(ctx, url, destination, 6)
}

func (c *Client) DownloadAll(ctx context.Context, outputDir string, items []model.DownloadItem, concurrency int) <-chan Result {
	results := make(chan Result)
	if concurrency < 1 {
		concurrency = 1
	}

	workCh := make(chan model.DownloadItem)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for item := range workCh {
			dest := filepath.Join(outputDir, filepath.FromSlash(item.Resource.RelativePath))
			err := c.FetchToFile(ctx, item.Resource.URL, dest)
			results <- Result{Item: item, Err: err}
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}

	go func() {
		defer close(workCh)
		for _, item := range items {
			select {
			case <-ctx.Done():
				return
			case workCh <- item:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func (c *Client) Stats() Stats {
	return c.limiter.Stats()
}

func (c *Client) fetchToFileWithRetry(ctx context.Context, url, destination string, attempts int) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			wait := retryDelay(lastErr, i)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		if err := c.pacer.Wait(ctx); err != nil {
			return err
		}
		lastErr = c.fetchToFile(ctx, url, destination)
		if lastErr == nil {
			c.pacer.NoteSuccess()
			c.limiter.NoteSuccess()
			return nil
		}
		if isThrottleError(lastErr) {
			c.pacer.NoteThrottle(lastErr)
			c.limiter.NoteThrottle()
		}
		if !isRetryableError(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func (c *Client) fetchToFile(ctx context.Context, url, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{
			StatusCode: resp.StatusCode,
			URL:        url,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	tmpFile := destination + ".part"
	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpFile)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpFile)
		return closeErr
	}

	if err := os.Rename(tmpFile, destination); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}

func isRetryableError(err error) bool {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		default:
			return statusErr.StatusCode >= 500
		}
	}
	return true
}

func retryDelay(err error, attempt int) time.Duration {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.RetryAfter > 0 {
		return statusErr.RetryAfter
	}

	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<min(attempt-1, 4)) * time.Second
	if delay > 15*time.Second {
		return 15 * time.Second
	}
	return delay
}

func isThrottleError(err error) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode == http.StatusServiceUnavailable
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 1 {
			return time.Second
		}
		if seconds > 30 {
			return 30 * time.Second
		}
		return time.Duration(seconds) * time.Second
	}

	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	delay := time.Until(when)
	if delay <= 0 {
		return time.Second
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func newAdaptivePacer(minInterval, maxCooldown time.Duration) *adaptivePacer {
	return &adaptivePacer{
		minInterval: minInterval,
		maxCooldown: maxCooldown,
	}
}

func (p *adaptivePacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	target := time.Now()
	target = maxTime(target, p.cooldownEnd)
	target = maxTime(target, p.nextRequest)
	if p.minInterval > 0 {
		p.nextRequest = target.Add(p.minInterval)
	} else {
		p.nextRequest = target
	}
	p.mu.Unlock()

	delay := time.Until(target)
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *adaptivePacer) NoteSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.penalty > 0 {
		p.penalty--
	}
	if p.penalty == 0 && time.Now().After(p.cooldownEnd) {
		p.cooldownEnd = time.Time{}
	}
}

func (p *adaptivePacer) NoteThrottle(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.penalty++
	if p.penalty > 6 {
		p.penalty = 6
	}

	delay := p.penaltyDelay(err)
	if delay > p.maxCooldown {
		delay = p.maxCooldown
	}

	cooldown := time.Now().Add(delay)
	if cooldown.After(p.cooldownEnd) {
		p.cooldownEnd = cooldown
	}
	if p.nextRequest.Before(p.cooldownEnd) {
		p.nextRequest = p.cooldownEnd
	}
}

func (p *adaptivePacer) penaltyDelay(err error) time.Duration {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.RetryAfter > 0 {
		return maxDuration(statusErr.RetryAfter, p.minInterval)
	}

	if p.penalty < 1 {
		return maxDuration(time.Second, p.minInterval)
	}
	delay := time.Duration(1<<min(p.penalty-1, 4)) * time.Second
	return maxDuration(delay, p.minInterval)
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func newAdaptiveConcurrency(maxConcurrency, minConcurrency int, enabled bool) *adaptiveConcurrency {
	return &adaptiveConcurrency{
		enabled: enabled,
		max:     maxConcurrency,
		min:     minConcurrency,
		current: maxConcurrency,
	}
}

func (a *adaptiveConcurrency) Acquire(ctx context.Context) error {
	for {
		a.mu.Lock()
		if a.inFlight < a.current {
			a.inFlight++
			a.mu.Unlock()
			return nil
		}
		a.mu.Unlock()

		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *adaptiveConcurrency) Release() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inFlight > 0 {
		a.inFlight--
	}
}

func (a *adaptiveConcurrency) NoteThrottle() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.successStreak = 0
	if !a.enabled || a.current <= a.min {
		return
	}
	a.current--
}

func (a *adaptiveConcurrency) NoteSuccess() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.enabled {
		return
	}
	a.successStreak++
	threshold := max(6, a.current*3)
	if a.current < a.max && a.successStreak >= threshold {
		a.current++
		a.successStreak = 0
	}
}

func (a *adaptiveConcurrency) Stats() Stats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Stats{
		EffectiveConcurrency: a.current,
		MaxConcurrency:       a.max,
		MinConcurrency:       a.min,
		InFlight:             a.inFlight,
		SuccessStreak:        a.successStreak,
		AutoTuneEnabled:      a.enabled,
	}
}
