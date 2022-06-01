package service

import (
	"context"
	"sync"
	"time"

	"captura-backup/internal/scheduler"
)

func (s *Service) dispatcherBackup(ctx context.Context, wgDispatchers *sync.WaitGroup) {
	wgDispatchers.Add(1)
	defer func() {
		wgDispatchers.Done()
		s.log.Warn("Backup dispatcher: stop dispatcher")
	}()
	s.log.Info("Backup dispatcher: start dispatcher")

	var jobs []*scheduler.Job

	startAutoBackup := make(chan *process)

SCHEDULER:
	for _, sch := range s.scheduler {
		workStart := sch.WorkStart.Format("15:04:05")
		prc := &process{
			DataID:  sch.DataID,
			Current: PRC_BACKUP,
		}
		s.log.Tracef("Backup dispatcher: try add to schedule dataID=%d interval=%s days_interval=%v time_start=%s", sch.DataID, sch.CheckInterval, sch.CheckIntervalDays, workStart)
		switch sch.CheckInterval {
		case "WEEK":
			switch len(sch.CheckIntervalDays) {
			case 0, 7:
				job, err := scheduler.Every().Day().At(workStart).Run(func() { startAutoBackup <- prc })
				if err != nil {
					s.log.Errorf("Backup dispatcher: could not add dataID:%d to the schedule for start backup: %s", prc.DataID, err)
					continue SCHEDULER
				}
				jobs = append(jobs, job)
			default:
				for _, day := range sch.CheckIntervalDays {
					job, err := scheduler.Every().DayOfWeek(day).At(workStart).Run(func() { startAutoBackup <- prc })
					if err != nil {
						s.log.Errorf("Backup dispatcher: could not add dataID:%d to the schedule for start backup: %s", prc.DataID, err)
						continue SCHEDULER
					}
					jobs = append(jobs, job)
				}
			}
		case "MONTH":
			if len(sch.CheckIntervalDays) == 0 {
				job, err := scheduler.Every().Day().At(workStart).Run(func() { startAutoBackup <- prc })
				if err != nil {
					s.log.Errorf("Backup dispatcher: could not add dataID:%d to the schedule for start backup: %s", prc.DataID, err)
					continue SCHEDULER
				}
				jobs = append(jobs, job)

			} else {
				for _, day := range sch.CheckIntervalDays {
					job, err := scheduler.Every().DayOfMonth(day).At(workStart).Run(func() { startAutoBackup <- prc })
					if err != nil {
						s.log.Errorf("Backup dispatcher: could not add dataID:%d to the schedule for start backup: %s", prc.DataID, err)
						continue SCHEDULER
					}
					jobs = append(jobs, job)
				}
			}
		}
	}

	if len(jobs) == 0 {
		s.log.Warn("Backup dispatcher: no data types have been added to the schedule: stop dispatcher")
		return
	}

	var wgProcess sync.WaitGroup
DISPATCHER:
	for {
		var (
			prc   *process
			start bool
		)
		select {
		case <-ctx.Done():
			s.log.Info("Backup dispatcher: recive cancel command: stop all schedule tasks")
			for _, job := range jobs {
				job.Quit <- true
			}
			break DISPATCHER
		case prc = <-s.startManualBackup:
			start = true
		case prc = <-startAutoBackup:
			start = true
		default:
			time.Sleep(1 * time.Second)
		}
		if start {
			wgProcess.Add(1)
			go s.backupProcess(ctx, prc, &wgProcess)
		}

	}
	s.log.Info("Backup dispatcher: waiting for all backup workers to finish their work")
	wgProcess.Wait()
}

func (s *Service) dispatcherRestore(ctx context.Context, wgDispatchers *sync.WaitGroup) {
	wgDispatchers.Add(1)
	s.log.Info("Restore dispatcher: start dispatcher")
	defer func() {
		defer wgDispatchers.Done()
		s.log.Warn("Restore dispatcher: stop dispatcher")
	}()
	var wgProcess sync.WaitGroup
DISPATCHER:
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Restore dispatcher: recive STOP command")
			break DISPATCHER
		case prc := <-s.startRestoreData:
			wgProcess.Add(1)
			go s.restoreProcess(ctx, prc, &wgProcess)
		default:
			time.Sleep(1 * time.Second)
		}
	}
	s.log.Info("Restore dispatcher: waiting for all restore workers to finish their work")
	wgProcess.Wait()
	// s.log.Info("Restore dispatcher: all recovery workers finish - stop dispatcher")
}

func (s *Service) dispatcherCleaning(ctx context.Context, wgDispatchers *sync.WaitGroup) {
	wgDispatchers.Add(1)
	s.log.Info("Cleaning dispatcher: start dispatcher")
	defer func() {
		wgDispatchers.Done()
		s.log.Warn("Cleaning dispatcher: stop dispatcher")
	}()
	var wgWorker sync.WaitGroup

	alarm := make(chan time.Time)
	signal := func() {
		alarm <- time.Now()
	}

	sig, err := scheduler.Every().Day().At("00:00:00").Run(signal)
	if err != nil {
		s.log.Errorln("Cleaning dispatcher: initialization of the work schedule:", err)
		return
	}
DISPATCHER:
	for {
		select {
		case <-ctx.Done():
			s.log.Info("Cleaning dispatcher: recive STOP command")
			sig.Quit <- true
			break DISPATCHER
		case <-time.After(time.Duration(24) * time.Hour):
			wgWorker.Add(1)
			go s.cleaningStorageProcess(ctx, &wgWorker)
		default:
			time.Sleep(5 * time.Second)
		}
	}
	s.log.Info("Cleaning dispatcher: waiting for all cleaning workers to finish their work")
	wgWorker.Wait()
	s.log.Info("Cleaning dispatcher: all cleaning workers finish")
}
