package service

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/storage"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (s *Service) cleaningStorageProcess(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		s.stateAndMessage(ctx,
			STATE_INACTIVE,
			"Service finished clean storage process at "+time.Now().Format("02.01.2006 15:04:05"),
		)
		wg.Done()
	}()

	producer, err := s.filesProducer()
	if err != nil {
		s.log.Errorln("Cleaning worker: get files producer for cleaning storage process:", err)
		return
	}
	defer producer.Close()

	storages, err := s.storer.StoragesForCleaner(ctx)
	if err != nil {
		s.log.Errorln("Cleaning worker: get list storages for clenaner:", err)
		return
	}

	storagePath := s.ini.Section("storage").Key("path").String()
	dirs, err := producer.ReadDir(storagePath)
	if err != nil {
		s.log.Errorln("Cleaning worker: read storages folder:", err)
		return
	}

	s.stateAndMessage(ctx,
		STATE_ACTIVE,
		"Service started clean storage process at "+time.Now().Format("02.01.2006 15:04:05"),
	)

	for _, st := range storages {
		keepDuration := time.Duration(st.KeepArchDays) * 24 * time.Hour
		for _, dir := range dirs {
			if dir.IsDir() {
				if dir.Name() == st.Schemaname {

					dates, err := producer.ReadDir(filepath.Join(storagePath, st.Schemaname))
					if err != nil {
						s.log.Errorln("Cleaning worker: read storages folder:", err)
						return
					}

					for _, date := range dates {
						day, err := time.Parse("20060102", date.Name())
						if err != nil {
							s.log.Error("Cleaning worker: parse storage dirs like a date:", err)
							continue
						}
						if time.Now().After(day.Add(keepDuration)) {

							if err := s.removeExpiredArchives(ctx, producer, st.Schemaname, date.Name()); err != nil {
								s.log.Errorln("Cleaning worker: remove expired archives:", err)
								continue
							}
						}
					}

				}
			}
		}
	}

	//TODO: Уведомление о завершении?
}

func (s *Service) removeExpiredArchives(ctx context.Context, producer storage.Producer, schema, date string) error {

	storagePath := s.ini.Section("storage").Key("path").String()
	files, err := producer.ReadDir(filepath.Join(storagePath, schema, date))
	if err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(storagePath, schema, date, file.Name())
		if err := producer.Remove(path); err != nil {
			return err
		}

		t := strings.Split(file.Name(), ".")
		if len(t) < 2 {
			return errors.New("wrong archive file name")
		}
		tbl := t[0]
		cntDate, err := time.Parse("20060102", date)
		if err != nil {
			return err
		}
		aad := datastructs.ArchAvailableData{
			SchemaName:  schema,
			ContentDate: cntDate,
			TableName:   tbl,
			DeletedAt:   time.Now(),
		}
		if err := s.storer.UpdateAvailableDataAfterRemoveFile(ctx, aad); err != nil {
			return err
		}
	}

	return nil
}
