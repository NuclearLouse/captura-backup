package service

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/storage"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"compress/gzip"
)

//Процессы восстановления могут быть одновременными, главное чтоб не шел процесс бэкапа для этого типа данных
func (s *Service) restoreProcess(ctx context.Context, prc *process, wgProcess *sync.WaitGroup) {
	defer func() {
		s.stateAndMessage(ctx,
			STATE_INACTIVE,
			"Service finished restore process at "+time.Now().Format("02.01.2006 15:04:05"),
		)
		wgProcess.Done()

	}()

	_, ok := s.processes.Load(prc.DataID)
	if ok {
		s.log.Warn("Restore process: the backup process for this data type is already in progress")
		return
	}

	for s.state == STATE_RELOAD {
		time.Sleep(1 * time.Second)
	}

	if s.isActiveProcess(PRC_BACKUP) {
		s.log.Warn("Restore process: the backup process is already in progress")
		return
	}

	s.processes.Store(prc.DataID, prc)
	defer s.processes.Delete(prc.DataID)

	s.stateAndMessage(ctx,
		STATE_ACTIVE,
		"Service started restore process at "+time.Now().Format("02.01.2006 15:04:05"),
	)

	//INFO: sc.TypeArchive может быть равно 0 - это для всех
	datas, err := s.storer.FilesForRestore(ctx, prc.DataID, prc.RestoreToDate)
	if err != nil {
		s.log.Errorln("Restore process: getting restore data:", err)
		return
	}
	if len(datas) == 0 {
		s.log.Trace("Restore process: no files to restore")
		return
	}

	producer, err := s.filesProducer()
	if err != nil {
		s.log.Errorln("Restore process: get files producer:", err)
		return
	}
	defer producer.Close()

	s.log.Infof("Restore process: [DataID:%d] start process", prc.DataID)
	defer func() {
		s.log.Infof("Restore process: [DataID:%d] finished process", prc.DataID)
	}()

	var wgWorker sync.WaitGroup
DATAS:
	for _, d := range datas {
		select {
		case <-ctx.Done():
			break DATAS
		default:
			s.buferWorkers <- struct{}{}
			wgWorker.Add(1)
			data := d
			go s.restoreWorker(ctx, data, producer, &wgWorker)
		}
	}
	wgWorker.Wait()
	//TODO: Уведомление о завершении?
}

func (s *Service) restoreWorker(ctx context.Context, data *datastructs.RestoreData, producer storage.Producer, wg *sync.WaitGroup) {
	defer func() {
		<-s.buferWorkers
		wg.Done()
		s.log.Infof("Restore worker: [DataID:%d Table:%s Date:%s] finished work", data.ID, data.TableName, data.ContentDate.Format("2006-01-02"))
	}()
	s.log.Infof("Restore worker: [DataID:%d Table:%s Date:%s] start work", data.ID, data.TableName, data.ContentDate.Format("2006-01-02"))

	if err := func() error {
		gzFile, err := producer.ReadFile(data.FileName)
		if err != nil {
			return fmt.Errorf("read archive GZ file: %w", err)
		}
		defer gzFile.Close()

		// filepath.Base(data.FileName)+".*" == RouteVKN10.backup.gz.2001064741
		///tmp/RouteVKN10.backup.gz.2001064741
		tmpGzFile, err := os.CreateTemp(
			s.ini.Section("service").Key("tmp_folder").String(),
			filepath.Base(data.FileName)+".*",
		)
		if err != nil {
			return fmt.Errorf("create tmpGZ file: %w", err)
		}
		defer func() {
			tmpGzFile.Close()
			os.Remove(tmpGzFile.Name())
		}()

		if _, err := io.Copy(tmpGzFile, gzFile); err != nil {
			return fmt.Errorf("copy archive GZ file into tmpGZ file: %w", err)
		}

		tmpGzFile.Close()

		tmpFile, err := os.Open(tmpGzFile.Name())
		if err != nil {
			return fmt.Errorf("create tmpGZ file: %w", err)
		}
		defer tmpFile.Close()

		gzReader, err := gzip.NewReader(tmpFile)
		if err != nil {
			return fmt.Errorf("read data into tmpGZ file: %w", err)
		}
		defer gzReader.Close()

		if err := s.storer.RestoreData(ctx, data, gzReader); err != nil {
			return fmt.Errorf("restore data from file into table: %w", err)
		}

		if err := s.storer.UpdateAvailableDataAfterRestoreFile(ctx, data.ID); err != nil {
			return fmt.Errorf("update table available data : %w", err)
		}

		return nil
	}(); err != nil {
		s.log.Errorf("Restore worker: [DataID:%d Table:%s Date: %s] process restore file: %s", data.ID, data.TableName, data.ContentDate.Format("2006-01-02"), err)
	}

}
