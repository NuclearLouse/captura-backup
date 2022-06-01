package store

import (
	"captura-backup/internal/datastructs"
	"context"
	"io"
	"os"
	"time"
)

// Storer ...
type Storer interface {

	//service and control
	ScheduleSettings(ctx context.Context) ([]*datastructs.ScheduleConfig, error)
	ResetStateControl(ctx context.Context) error
	StateAndMessageService(ctx context.Context, state, message string) error
	StateControl(ctx context.Context) (*datastructs.ControlPanel, error)

	//backup process
	DatasToArchive(ctx context.Context, id int) ([]*datastructs.ArchiveTable, error)
	DatesToBackup(ctx context.Context, data *datastructs.ArchiveTable) ([]time.Time, error)
	WasRestoredAndExpired(ctx context.Context, data datastructs.ArchAvailableData) (bool, error)
	SaveDataForDay(ctx context.Context, data *datastructs.ArchiveTable, day time.Time, tmpFile *os.File) (int64, error)
	DeleteDataForDay(ctx context.Context, data *datastructs.ArchiveTable, day time.Time, rowsSave int64) error
	AddArchAvailableData(ctx context.Context, data datastructs.ArchAvailableData) error
	DeleteTable(ctx context.Context, table string) error

	//storage clean process
	StoragesForCleaner(ctx context.Context) ([]datastructs.ArchiveStorage, error)
	UpdateAvailableDataAfterRemoveFile(ctx context.Context, data datastructs.ArchAvailableData) error
	
	//restore process
	FilesForRestore(ctx context.Context, archive int, date time.Time) ([]*datastructs.RestoreData, error)
	RestoreData(ctx context.Context, data *datastructs.RestoreData, tmpFile io.Reader) error
	UpdateAvailableDataAfterRestoreFile(ctx context.Context, id int) error

}
