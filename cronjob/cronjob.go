package cronjob

import (
	"fmt"
	"sync"
	"time"

	pkgzap "github.com/hydroan/gst/logger/zap"
	"github.com/hydroan/gst/util"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var (
	c        *cron.Cron
	log      *pkgzap.Logger
	cronjobs = make([]*cronjob, 0)
	parser   cron.Parser
	mu       sync.Mutex

	inited bool
)

type cronjob struct {
	name           string
	spec           string
	fn             func() error
	sched          cron.Schedule
	runImmediately bool
}

// Config defines the configuration for cronjob package
type Config struct {
	// RunImmediately indicates whether to run the cronjob immediately after registration
	// in addition to the scheduled execution
	RunImmediately bool `json:"run_immediately" yaml:"run_immediately" toml:"run_immediately"`
}

func init() {
	parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

func Init() (err error) {
	if log == nil {
		log = pkgzap.New("cronjob.log")
	}
	if c == nil {
		c = cron.New(cron.WithSeconds())
	}

	for _, cj := range cronjobs {
		register(cj)
	}

	c.Start()

	inited = true
	return nil
}

// Register cronjob can be called at any point before or after Init().
// The config parameter is optional and can be used to customize cronjob behavior.
func Register(fn func() error, spec string, name string, config ...Config) {
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	}

	mu.Lock()
	defer mu.Unlock()
	cj := &cronjob{
		name:           name,
		spec:           spec,
		fn:             fn,
		runImmediately: cfg.RunImmediately,
	}

	if inited {
		register(cj)
	} else {
		cronjobs = append(cronjobs, cj)
	}
}

func register(cj *cronjob) {
	var err error
	if cj == nil {
		return
	}
	if cj.spec == "" {
		return
	}
	if cj.sched, err = parser.Parse(cj.spec); err != nil {
		log.Errorz(fmt.Sprintf("failed to parse cronjob spec: %s", err), zap.String("name", cj.name), zap.String("spec", cj.spec))
		return
	}

	// Execute immediately if configured to do so
	if cj.runImmediately {
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorw(fmt.Sprintf("cronjob immediate execution panic: %s", err), "name", cj.name, "spec", cj.spec)
				}
			}()
			begin := time.Now()
			if err = cj.fn(); err != nil {
				log.Errorz(fmt.Sprintf("finished immediate cronjob execution with error: %s", err), zap.String("name", cj.name), zap.String("spec", cj.spec), zap.String("cost", util.FormatDurationSmart(time.Since(begin))))
			} else {
				log.Infoz("finished immediate cronjob execution", zap.String("name", cj.name), zap.String("spec", cj.spec), zap.String("cost", util.FormatDurationSmart(time.Since(begin))))
			}
		}()
	}

	if _, err = c.AddFunc(cj.spec, func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorw(fmt.Sprintf("cronjob panic: %s", err), "name", cj.name, "spec", cj.spec)
			}
		}()
		begin := time.Now()
		if err = cj.fn(); err != nil {
			log.Errorz(fmt.Sprintf("finished cronjob with error: %s", err), zap.String("name", cj.name), zap.String("spec", cj.spec), zap.Time("next", cj.sched.Next(begin)), zap.String("cost", util.FormatDurationSmart(time.Since(begin))))
		} else {
			log.Infoz("finished cronjob", zap.String("name", cj.name), zap.String("spec", cj.spec), zap.Time("next", cj.sched.Next(begin)), zap.String("cost", util.FormatDurationSmart(time.Since(begin))))
		}
	}); err != nil {
		log.Errorz(fmt.Sprintf("failed to add cronjob: %s", err), zap.String("name", cj.name), zap.String("spec", cj.spec))
	} else {
		log.Infoz("successfully add cronjob", zap.String("name", cj.name), zap.String("spec", cj.spec), zap.Bool("run_immediately", cj.runImmediately))
	}
}
