package service

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/storage"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (s *Service) backupProcess(ctx context.Context, prc *process, wgProcess *sync.WaitGroup) {
	defer func() {
		s.stateAndMessage(ctx,
			STATE_INACTIVE,
			"Service finished backup process at "+time.Now().Format("02.01.2006 15:04:05"),
		)
		wgProcess.Done()

	}()

	_, ok := s.processes.Load(prc.DataID)
	if ok {
		s.log.Warn("Backup process: the backup process for this data type is already in progress")
		return
	}

	for s.state == STATE_RELOAD {
		time.Sleep(1 * time.Second)
	}

	if s.isActiveProcess(PRC_RESTORE) {
		s.log.Warn("Backup process: the recovery process is already in progress")
		return
	}

	s.processes.Store(prc.DataID, prc)
	defer s.processes.Delete(prc.DataID)

	s.stateAndMessage(ctx,
		STATE_ACTIVE,
		"Service started backup process at "+time.Now().Format("02.01.2006 15:04:05"),
	)

	datas, err := s.storer.DatasToArchive(ctx, prc.DataID)
	if err != nil {
		s.log.Errorln("Backup process: getting backup data:", err)
		return
	}
	if len(datas) == 0 {
		return
	}

	producer, err := s.filesProducer()
	if err != nil {
		s.log.Errorln("Backup process: get files producer:", err)
		return
	}
	defer producer.Close()

	s.log.Infof("Backup process: [DataID:%d] start process", prc.DataID)
	defer func() {
		s.log.Infof("Backup process: [DataID:%d] finished process", prc.DataID)
	}()

	var wgWorker sync.WaitGroup
DATAS:
	for _, d := range datas {
		select {
		case <-ctx.Done():
			//прервать или выйти?
			break DATAS
		default:
			s.buferWorkers <- struct{}{}
			wgWorker.Add(1)
			data := d
			go s.backupWorker(ctx, data, producer, &wgWorker)
		}

	}
	wgWorker.Wait()
	//TODO: Уведомление о завершении ?
}

func (s *Service) backupWorker(ctx context.Context, data *datastructs.ArchiveTable, producer storage.Producer, wg *sync.WaitGroup) {

	defer func() {
		<-s.buferWorkers
		wg.Done()
		s.log.Infof("Backup worker: [DataID:%d Table:%s Entity:%s] finished work", data.ID, data.Name, data.Entity)
	}()
	s.log.Infof("Backup worker: [DataID:%d Table:%s Entity:%s] start work", data.ID, data.Name, data.Entity)

	if err := func() error {
		dates, err := s.storer.DatesToBackup(ctx, data)
		if err != nil {
			return fmt.Errorf("getting backup dates: %w", err)
		}

		if len(dates) == 0 {
			s.log.Tracef("Backup worker: [DataID:%d Table:%s Entity:%s] no dates for backup", data.ID, data.Name, data.Entity)
			return nil
		}

		schemaTbl := strings.Split(data.Name, ".")
		if len(schemaTbl) < 2 {
			return errors.New("wrong schema-table name for data backup")
		}

		storageFolder := filepath.Join(s.ini.Section("storage").Key("path").String(), schemaTbl[0])

		if err := producer.MakeDir(storageFolder); err != nil {
			return fmt.Errorf("create storage folder: %w", err)
		}

		for _, day := range dates {
			var rowsSave int64
			if data.DoBackup {

				stats := datastructs.ArchAvailableData{
					DataID:          data.ID,
					SchemaName:      schemaTbl[0],
					TableName:       schemaTbl[1],
					SingleTable:     data.Entity == "table",
					ContentDate:     day,
					RestoreTemplate: data.RestoreTemplate,
				}

				ok, err := s.storer.WasRestoredAndExpired(ctx, stats)
				if err != nil {
					return fmt.Errorf("verification of data for the former recovery: %w", err)
				}
				if !ok {
					s.log.Warnf("Backup worker: [Table:%s Date:%s] the backup has been cancelled, the data has been restored and the retention period has not expired yet",
						data.Name,
						day.Format("2006-01-02"))
					continue
				}

				// /tmp/RouteVKN10.1257894000000000000
				// fileName := removeQuotes(schemaTbl[1])
				fileName := func(text string) string {
					return strings.NewReplacer(`"`, "").Replace(text)
				}(schemaTbl[1])

				tmpGzFile, err := os.CreateTemp(
					s.ini.Section("service").Key("tmp_folder").String(),
					fileName+".*",
				)
				if err != nil {
					return fmt.Errorf("create tmpGz file: %w", err)
				}
				defer func() {
					tmpGzFile.Close()
					os.Remove(tmpGzFile.Name())
				}()

				rowsSave, err = s.storer.SaveDataForDay(ctx, data, day, tmpGzFile)
				if err != nil {
					return fmt.Errorf("save backup data: %w", err)
				}

				tmpGzFile.Close()

				gzReader, err := os.Open(tmpGzFile.Name())
				if err != nil {
					return fmt.Errorf("read tmpGz file data: %w", err)
				}
				defer gzReader.Close()

				path := filepath.Join(storageFolder, day.Format("20060102"))
				if err := producer.MakeDir(path); err != nil {
					return fmt.Errorf("create storage folder for backup day: %w", err)
				}

				absFileName := filepath.Join(path, fileName+".backup.gz")
				if err := producer.SaveFile(absFileName, gzReader); err != nil {
					return fmt.Errorf("copy tmp gzFile to storage: %w", err)
				}

				gzReader.Close()
				stats.FileName = absFileName
				stats.ArchivedAt = time.Now()
				stats.ContentRows = rowsSave

				if err := s.storer.AddArchAvailableData(ctx, stats); err != nil {
					return fmt.Errorf("add statistics for arch available data: %w", err)
				}
			}

			if !s.developMode() {
				err := s.storer.DeleteDataForDay(ctx, data, day, rowsSave)
				if err != nil {
					return fmt.Errorf("delete data from table after backup: %w", err)
				}
			}

			s.log.Infof("Backup worker: [DataID:%d Table:%s Entity:%s] successful backup and delete data for Day:%s CountRows:%d",
				data.ID, data.Name, data.Entity, day.Format("2006-01-02"), rowsSave)
		}

		if data.Entity == "table" {
			if !s.developMode() {
				if err := s.storer.DeleteTable(ctx, data.Name); err != nil {
					return fmt.Errorf("delete table: %w", err)
				}
			}
		}
		return nil
	}(); err != nil {
		s.log.Errorf("Backup worker: [DataID:%d Table:%s Entity:%s] process archive file:%s", data.ID, data.Name, data.Entity, err)
	}
}
