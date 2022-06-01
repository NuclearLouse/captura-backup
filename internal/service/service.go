package service

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/logger"
	"captura-backup/internal/notification"
	"captura-backup/internal/store"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

var version, configFolder string

const appConfig = "service.conf"

type state int

const (
	STATE_UNKNOWN state = iota
	STATE_ACTIVE
	STATE_INACTIVE
	STATE_RELOAD
	STATE_STOPPED
)

type Service struct {
	state             state
	storer            store.Storer
	log               *logrus.Logger
	ini               *ini.File
	scheduler         []*datastructs.ScheduleConfig
	senders           []notification.Notificator
	stop              chan *stopContext
	reloadCfg         chan struct{}
	buferWorkers      chan struct{}
	startManualBackup chan *process
	startRestoreData  chan *process
	//INFO: the map into which the current processes are written, the key is the data ID, and the value is the *process structure
	processes sync.Map
	// INFO: stores a list of errors, where the key is the error itself, and the UNIX value the time when it occured
	journalLogs sync.Map
}

type stopContext struct {
	fromUI bool
}

func Version() {
	fmt.Println("Version=", version)
}

func New() (*Service, error) {
	dir, err := os.Open(configFolder)
	if err != nil {
		return nil, fmt.Errorf("open config folder: %w", err)
	}
	files, err := dir.Readdirnames(0)
	if err != nil {
		return nil, fmt.Errorf("read config folder: %w", err)
	}

	var addConfigs []interface{}
	for _, file := range files {
		if file == appConfig {
			continue
		}
		addConfigs = append(addConfigs, filepath.Join(configFolder, file))
	}

	cfg, err := ini.LoadSources(ini.LoadOptions{
		SpaceBeforeInlineComment: true,
	},
		filepath.Join(configFolder, appConfig),
		addConfigs...,
	)
	if err != nil {
		return nil, fmt.Errorf("load config files: %w", err)
	}
	cfg.NameMapper = ini.TitleUnderscore

	cfgLog := logger.DefaultConfig()
	if err := cfg.Section("logger").MapTo(cfgLog); err != nil {
		return nil, fmt.Errorf("mapping logger config: %w", err)
	}
	return &Service{
		ini:       cfg,
		log:       logger.New(cfgLog),
		stop:      make(chan *stopContext, 1),
		reloadCfg: make(chan struct{}, 1),
		buferWorkers: make(chan struct{},
			cfg.Section("service").Key("limit_workers").MustInt(5)),
		startManualBackup: make(chan *process, 1),
		startRestoreData:  make(chan *process, 1),
	}, nil
}

func (s *Service) checkStorageFolder() error {
	storageFolder := s.ini.Section("storage").Key("path").String()
	s.log.Traceln("Servcie: check if exists storage folder:", storageFolder)
	producer, err := s.filesProducer()
	if err != nil {
		return fmt.Errorf("get files producer: %w", err)
	}
	defer producer.Close()
	return producer.MakedirAll(storageFolder)
}

func (s *Service) stateAndMessage(ctx context.Context, state state, message ...string) {
	s.state = state
	mess := "NULL"
	if message != nil {
		mess = message[0]
	}
	if err := s.storer.StateAndMessageService(ctx, s.currentState(), mess); err != nil {
		s.log.Errorf("Service: could not write message and update service status: %s", err)
	}
}

type prc int

const (
	PRC_NOT_OR_UNKNOWN prc = iota
	PRC_BACKUP
	PRC_RESTORE
)

type process struct {
	DataID        int
	Current       prc
	RestoreToDate time.Time
}

func (p process) name() string {
	switch p.Current {
	case PRC_BACKUP:
		return "BACKUP"
	case PRC_RESTORE:
		return "RESTORE"
	}
	return "NOT OR UNKNOWN"
}

func (s *Service) currentState() string {
	switch s.state {
	case STATE_STOPPED:
		return "Service stopped..."
	case STATE_INACTIVE:
		return "Service is running but the workers are not active ..."
	case STATE_RELOAD:
		return "Service reloads configuration ..."
	case STATE_ACTIVE:
		return fmt.Sprintf("Service is active with the following processes:[%s]", func() string {
			var (
				prcs []string
			)
			s.processes.Range(func(key, value interface{}) bool {
				p := value.(*process)
				if p.Current != PRC_NOT_OR_UNKNOWN {
					prcs = append(prcs, p.name())
				}
				return true
			})
			return strings.Join(prcs, "|")
		}())
	}
	return "WARNING! UNKNOWN SERVICE STATE!"
}

func (s *Service) isActiveProcess(prc prc) bool {
	var is bool
	s.processes.Range(func(key, value interface{}) bool {
		p := value.(*process)
		if p.Current == prc {
			is = true
			return false
		}
		return true
	})
	return is
}

func (s *Service) clearProcesses() {
	s.processes.Range(func(key, value interface{}) bool {
		s.processes.Delete(key)
		return true
	})
}

func (s *Service) makeDataToSend(templ, text string, err ...string) notification.DataToSend {
	return notification.DataToSend{
		Header: fmt.Sprintf("%s: %s, %s",
			strings.ToUpper(templ),
			s.ini.Section("service").Key("server_name").String(),
			s.ini.Section("service").Key("service_name").String(),
		),
		Server:   s.ini.Section("service").Key("server_name").String(),
		Service:  s.ini.Section("service").Key("service_name").String(),
		Text:     text,
		Errors:   err,
		Template: templ,
	}
}

func (s *Service) sendMessage(data notification.DataToSend) error {
	for _, s := range s.senders {

		if err := s.SendMessage(data); err != nil {
			return err
		}
	}
	return nil
}

type log struct {
	key      string
	lvl      string
	ctx      string
	msg      string
	err      error
	function func()
}

// Inside all infinite loops where the log message can spam the log file
func (s *Service) handleLogs(l log) {
	timeNow := time.Now().Unix()

	result := func() bool {
		if lastLog, ok := s.journalLogs.Load(l.key); ok {
			return !((timeNow - lastLog.(int64)) <= s.ini.Section("service").Key("log_spam_period").MustInt64(60))
		}
		return true
	}()
	var slog func(format string, args ...interface{}) = s.log.Errorf
	if result {
		s.journalLogs.Store(l.key, timeNow)
		switch l.lvl {
		case "trace":
			slog = s.log.Tracef
		case "debug":
			slog = s.log.Debugf
		case "info":
			slog = s.log.Infof
		case "warn":
			slog = s.log.Warnf
		}
		if l.err != nil {
			slog("%s: %s: %s", l.ctx, l.msg, l.err)
		} else {
			slog("%s: %s", l.ctx, l.msg)
		}
		if l.function != nil {
			go l.function()
		}
	}
}

func (s *Service) developMode() bool {
	return s.ini.Section("service").Key("develop_mode").MustBool(false)
}