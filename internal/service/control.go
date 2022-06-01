package service

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/store/postgres"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func (s *Service) Start() {

	s.log.Infof("***********************SERVICE [v%s] START***********************", version)

	if err := s.checkStorageFolder(); err != nil {
		s.log.Fatalln("Service: could not create archive storage directory:", err)
	}

	ctx, globCancel := context.WithCancel(context.Background())
	defer globCancel()

	pool, err := s.connectDatabase(ctx)
	if err != nil {
		s.log.Fatalln("Service: database connect:", err)
	}
	s.storer = postgres.New(pool, s.ini)

	if err := s.storer.ResetStateControl(ctx); err != nil {
		s.log.Fatalln("Service: reset state control:", err)
	}

	go s.clearProcesses()

	s.notificators()

	s.newScheduler(ctx)

	s.stateAndMessage(ctx,
		STATE_INACTIVE,
		"Service start at "+time.Now().Format("02.01.2006 15:04:05"),
	)

	var wgDispatchers sync.WaitGroup
	ctxWithSchedule, cancelSchedul := context.WithCancel(ctx)

	go s.monitorEventUI(ctx)
	go s.monitorSignalOS(ctx)

	go s.dispatcherBackup(ctxWithSchedule, &wgDispatchers)
	go s.dispatcherRestore(ctx, &wgDispatchers)
	go s.dispatcherCleaning(ctx, &wgDispatchers)

	go func() {
		if err := s.sendMessage(s.makeDataToSend("test", "")); err != nil {
			s.log.Errorln("Service: send test notification:", err)
		}
	}()

	var sCtx *stopContext
CONTROL:
	for {
		select {
		case sCtx = <-s.stop:
			break CONTROL
		case <-s.reloadCfg:
			oldState := s.state
			s.stateAndMessage(ctx,
				STATE_RELOAD,
				"Service started reload config at "+time.Now().Format("02.01.2006 15:04:05"),
			)
			cancelSchedul()
			ctxWithSchedule, cancelSchedul = context.WithCancel(ctx)
			s.newScheduler(ctxWithSchedule)
			s.stateAndMessage(ctx,
				oldState,
				"Service ended reload config at "+time.Now().Format("02.01.2006 15:04:05"),
			)
			go s.dispatcherBackup(ctxWithSchedule, &wgDispatchers)
		default:
			time.Sleep(1 * time.Second)
		}
	}

	s.stateAndMessage(ctx,
		STATE_STOPPED,
		"Service stop at "+time.Now().Format("02.01.2006 15:04:05"),
	)

	cancelSchedul()
	globCancel()
	
	s.log.Info("Service: waiting for all dispatchers to finish their work")
	wgDispatchers.Wait()

	s.log.Info("Service: all dispatchers finish work")
	// pool.Close()
	
	s.log.Warn("Service: disconnect to database")

	defer func() {
		s.log.Info("***********************SERVICE STOP************************")
	}()

	if sCtx.fromUI {
		s.log.Warn("Service: trying to stop the service daemon using systemctl")
		s.Stop()
	}

}

func (s *Service) newScheduler(ctx context.Context) {
	scheduler, err := s.storer.ScheduleSettings(ctx)
	if err != nil {
		s.log.Errorln("Service: get scheduler settings:", err)
		scheduler = datastructs.DefaultScheduleConfig()
		go func() {
			if err := s.sendMessage(
				s.makeDataToSend(
					"error",
					"Could not read the schedule settings, the default schedule was applied",
					err.Error()),
			); err != nil {
				s.log.Errorln("Service: send test notification:", err)
			}
		}()

	}
	s.scheduler = scheduler
}

func (s *Service) monitorEventUI(ctx context.Context) {
	s.log.Info("EventUI monitor: start monitoring")

	for {
		select {
		case <-ctx.Done():
			s.log.Warn("EventUI monitor: stop monitoring")
			return
		default:
			sc, err := s.storer.StateControl(ctx)
			if err != nil {
				// s.log.Errorln("EventUI monitor: control state in the user interface:", err)
				s.handleLogs(log{
					lvl: "err",
					ctx: "EventUI monitor",
					key: "eventui-read-panel",
					msg: "read control panel state",
					err: err,
				})
				return
			}
			switch {
			case sc.StopService:
				s.log.Info("EventUI monitor: recive STOP command")
				s.stop <- &stopContext{fromUI: true}
				s.log.Warn("EventUI monitor: stop monitoring")
				return
			case sc.StartRecovery:
				s.log.Info("EventUI monitor: recive START RESTORE command")
				s.startRestoreData <- &process{
					DataID:        sc.TypeArchive,
					RestoreToDate: sc.RestoreToThisDate,
					Current:       PRC_RESTORE,
				}
				err = s.storer.ResetStateControl(ctx)
			case sc.ReloadConfig:
				s.log.Info("EventUI monitor: recive RELOAD CONFIG command")
				s.reloadCfg <- struct{}{}
				err = s.storer.ResetStateControl(ctx)
			case sc.StartManualBackup:
				s.log.Info("EventUI monitor: recive START MANUAL BACKUP command")
				s.startManualBackup <- &process{
					DataID:  sc.TypeArchive,
					Current: PRC_BACKUP,
				}
				err = s.storer.ResetStateControl(ctx)
			}
			if err != nil {
				s.handleLogs(log{
					lvl: "err",
					ctx: "EventUI monitor",
					key: "eventui-reset-panel",
					msg: "reset state control panel",
					err: err,
				})
			}
		}
		time.Sleep(2 * time.Second)
	}

}

func (s *Service) monitorSignalOS(ctx context.Context) {
	s.log.Info("SignalOS monitor: start monitoring")
	defer func() {
		s.log.Warn("SignalOS monitor: stop monitoring")
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit,
		os.Interrupt,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGABRT)
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-quit:
			s.log.Infof("SignalOS monitor: signal recived: %v", sig)
			s.stop <- &stopContext{}
			return
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (s *Service) Stop() {
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		s.log.Errorf("Service: check root permisions: %s", err)
		return
	}
	// 0 = root, 501 = non-root user
	i, err := strconv.Atoi(string(out[:len(out)-1]))
	if err != nil {
		s.log.Errorf("Service: convert answer check root: %s", err)
		return
	}
	type bash struct {
		apps string
		args []string
	}
	var command bash
	switch i {
	case 0:
		command = bash{
			apps: "systemctl",
			args: []string{"stop",
				s.ini.Section("service").Key("daemon_file").MustString("captura-backup.service")},
		}
	default:
		// echo 'PASSWORD' | sudo -S systemctl stop captura-backup.service
		command = bash{
			apps: "echo",
			args: []string{
				fmt.Sprintf("'%s'", s.ini.Section("server").Key("sudo_pass").String()),
				"|",
				"sudo",
				"-S",
				"systemctl",
				"stop",
				s.ini.Section("service").Key("daemon_file").MustString("captura-backup.service"),
			},
		}
	}

	if err := exec.Command(command.apps, command.args...).Run(); err != nil {
		s.log.Errorf("Service: exec command: %s", err)
		return
	}
}
