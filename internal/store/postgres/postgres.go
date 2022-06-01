package postgres

import (
	"captura-backup/internal/datastructs"
	"captura-backup/internal/store"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"gopkg.in/ini.v1"
)

type Store struct {
	*pgxpool.Pool
	ini *ini.File
}

func New(pool *pgxpool.Pool, ini *ini.File) store.Storer {
	return &Store{pool, ini}
}

func (db *Store) pgEntity(entity, key string) string {
	return fmt.Sprintf(" %s.%s ",
		db.ini.Section("database").Key("schema").String(),
		db.ini.Section("database."+entity).Key(key).String(),
	)
}

func (db *Store) StateAndMessageService(ctx context.Context, state, message string) error {
	_, err := db.Exec(ctx,
		"SELECT FROM"+db.pgEntity("function", "state_and_message")+"($1,$2)", state, message)
	return err
}

func (db *Store) ResetStateControl(ctx context.Context) error {
	_, err := db.Exec(ctx,
		"SELECT FROM"+db.pgEntity("function", "reset_state_control")+"()")
	return err
}

func (db *Store) StateControl(ctx context.Context) (*datastructs.ControlPanel, error) {
	var (
		cp       datastructs.ControlPanel
		archive  pgtype.Int4
		thisDate pgtype.Date
	)
	if err := db.QueryRow(ctx,
		`SELECT stop_manager, reload_config, start_recovery, restore_to_this_date, start_manual_backup, type_archive
		FROM`+db.pgEntity("table", "service_control")).
		Scan(
			&cp.StopService,
			&cp.ReloadConfig,
			&cp.StartRecovery,
			&thisDate,
			&cp.StartManualBackup,
			&archive,
		); err != nil {
		return nil, err
	}
	if archive.Status != pgtype.Null {
		cp.TypeArchive = int(archive.Int)
	}
	if thisDate.Status != pgtype.Null {
		cp.RestoreToThisDate = thisDate.Time
	}

	return &cp, nil
}

func (db *Store) ScheduleSettings(ctx context.Context) ([]*datastructs.ScheduleConfig, error) {
	var scheduler []*datastructs.ScheduleConfig
	rows, err := db.Query(ctx,
		`SELECT data_id, check_interval, check_interval_days, work_start 
		FROM`+db.pgEntity("table", "schedule_settings")+"WHERE enabled = TRUE",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sch datastructs.ScheduleConfig
		if err := rows.Scan(
			&sch.DataID,
			&sch.CheckInterval,
			&sch.CheckIntervalDays,
			&sch.WorkStart,
		); err != nil {
			return nil, err
		}
		scheduler = append(scheduler, &sch)
	}
	return scheduler, nil
}

func (db *Store) DatasToArchive(ctx context.Context, id int) ([]*datastructs.ArchiveTable, error) {
	var ats []*datastructs.ArchiveTable
	rows, err := db.Query(ctx,
		"SELECT * FROM"+db.pgEntity("function", "datas_to_archive")+"($1);", id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			at                                                  datastructs.ArchiveTable
			entity, dateclmn, condfmt, rminterval, restTemplate pgtype.Varchar
			dobackup                                            pgtype.Bool
			keepRestore                                         pgtype.Int4
		)
		if err := rows.Scan(
			&at.Name,
			&at.ID,
			&entity,
			&dateclmn,
			&condfmt,
			&rminterval,
			&dobackup,
			&restTemplate,
			&keepRestore,
		); err != nil {
			return nil, err
		}

		if entity.Status != pgtype.Null {
			at.Entity = entity.String
		}
		if dateclmn.Status != pgtype.Null {
			at.DateColumn = dateclmn.String
		}
		if condfmt.Status != pgtype.Null {
			at.DateColumn = dateclmn.String
		}
		if rminterval.Status != pgtype.Null {
			at.RmInterval = rminterval.String
		}
		if dobackup.Status != pgtype.Null {
			at.DoBackup = dobackup.Bool
		}
		if restTemplate.Status != pgtype.Null {
			at.RestoreTemplate = restTemplate.String
		}
		if keepRestore.Status != pgtype.Null {
			at.KeepRestore = int(keepRestore.Int)
		}

		ats = append(ats, &at)
	}
	return ats, nil
}

func (db *Store) DatesToBackup(ctx context.Context, data *datastructs.ArchiveTable) ([]time.Time, error) {
	// SELECT DISTINCT "ST_Date" FROM billdb_rec.rec_6_40_cdr WHERE "ST_Date" < (current_date - '6 MONTH'::INTERVAL)::DATE
	// SELECT DISTINCT DateColumn FROM Name WHERE DateColumn < (current_date - RmInterval::INTERVAL)::DATE
	var dates []time.Time
	sql := fmt.Sprintf(`SELECT DISTINCT %s FROM %s WHERE %s < (current_date - '%s'::INTERVAL)::DATE`, data.DateColumn, data.Name, data.DateColumn, data.RmInterval)
	rows, err := db.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	return dates, nil
}

func (db *Store) SaveDataForDay(ctx context.Context, data *datastructs.ArchiveTable, day time.Time, tmpFile *os.File) (int64, error) {
	gzWriter := gzip.NewWriter(tmpFile)
	defer gzWriter.Close()

	con, err := db.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire connection from pgpool: %w", err)
	}
	defer con.Release()

	query := fmt.Sprintf(
		`COPY (SELECT * FROM %s WHERE %s = '%s') TO STDOUT WITH CSV NULL 'NULL' DELIMITER ';' HEADER ;`,
		data.Name,
		data.DateColumn,
		day.Format("2006-01-02"))

	ctxCopy, cancel := context.WithTimeout(ctx, time.Duration(5*time.Minute))
	defer cancel()
	tag, err := con.Conn().PgConn().CopyTo(ctxCopy, gzWriter, query)
	if err != nil {
		return 0, fmt.Errorf("copy data to file from table: %w", err)
	}

	return tag.RowsAffected(), nil
}

func (db *Store) DeleteDataForDay(ctx context.Context, data *datastructs.ArchiveTable, day time.Time, rowsSave int64) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction:%w", err)
	}
	defer tx.Rollback(ctx)

	query := fmt.Sprintf(`DELETE FROM %s WHERE %s = '%s'`,
		data.Name,
		data.DateColumn,
		day.Format("2006-01-02"))
	tag, err := tx.Exec(ctx, query)

	if err != nil {
		return fmt.Errorf("execute transaction: %w", err)
	}
	if tag.RowsAffected() != rowsSave {
		return errors.New("the number of saved and deleted records do not match")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (db *Store) DeleteTable(ctx context.Context, table string) (err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction:%w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
		return fmt.Errorf("execute transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (db *Store) AddArchAvailableData(ctx context.Context, d datastructs.ArchAvailableData) error {
	var err error
	if d.RestoreTemplate != "" {
		_, err = db.Exec(ctx,
			"SELECT FROM"+db.pgEntity("function", "add_available_data")+"($1,$2,$3,$4,$5,$6,$7,$8,$9);",
			d.DataID,
			d.SchemaName,
			d.TableName,
			d.SingleTable,
			d.FileName,
			d.ContentDate,
			d.ContentRows,
			d.ArchivedAt,
			d.RestoreTemplate,
		)
	} else {
		_, err = db.Exec(ctx,
			"SELECT FROM"+db.pgEntity("function", "add_available_data")+"($1,$2,$3,$4,$5,$6,$7,$8,NULL);",
			d.DataID,
			d.SchemaName,
			d.TableName,
			d.SingleTable,
			d.FileName,
			d.ContentDate,
			d.ContentRows,
			d.ArchivedAt,
		)
	}

	return err
}

func (db *Store) StoragesForCleaner(ctx context.Context) ([]datastructs.ArchiveStorage, error) {
	rows, err := db.Query(ctx,
		"SELECT id, schemaname, keep_arch_days FROM"+db.pgEntity("table", "config_table_list")+"WHERE keep_arch_days > 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ass []datastructs.ArchiveStorage
	for rows.Next() {
		var as datastructs.ArchiveStorage
		if err := rows.Scan(
			&as.DataID,
			&as.Schemaname,
			&as.KeepArchDays,
		); err != nil {
			return nil, err
		}
		ass = append(ass, as)
	}
	return ass, nil
}

func (db *Store) UpdateAvailableDataAfterRemoveFile(ctx context.Context, data datastructs.ArchAvailableData) error {
	_, err := db.Exec(ctx,
		"UPDATE"+db.pgEntity("table", "available_data")+"SET deleted_at=$1 WHERE schemaname=$2 AND tblname=$3 AND content_date=$4",
		data.DeletedAt,
		data.SchemaName,
		data.TableName,
		data.ContentDate,
	)
	return err
}

func (db *Store) WasRestoredAndExpired(ctx context.Context, data datastructs.ArchAvailableData) (bool, error) {
	var expired bool
	err := db.QueryRow(ctx,
		`SELECT (COALESCE(aad.restored_at, to_timestamp(0))::date + ctl.keep_restore_days) < now()::date
		FROM`+db.pgEntity("table", "available_data")+`aad 
		JOIN`+db.pgEntity("table", "config_table_list")+`ctl ON ctl.id = aad.data_id
		WHERE aad.schemaname=$1 AND aad.tblname=$2 AND aad.content_date=$3`,
		data.SchemaName,
		data.TableName,
		data.ContentDate,
	).Scan(&expired)
	if err != nil && err == pgx.ErrNoRows {
		return true, nil
	}
	return expired, err
}

func (db *Store) FilesForRestore(ctx context.Context, dataID int, date time.Time) ([]*datastructs.RestoreData, error) {
	var data []*datastructs.RestoreData
	rows, err := db.Query(ctx,
		"SELECT * FROM"+db.pgEntity("function", "files_for_restore")+"($1,$2);", dataID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			d                          datastructs.RestoreData
			restoreTempl, currentTempl pgtype.Varchar
		)
		if err := rows.Scan(
			&d.ID,
			&d.DataID,
			&d.FileName,
			&d.SchemaName,
			&d.TableName,
			&d.SingleTable,
			&d.ContentDate,
			&d.ContentRows,
			&restoreTempl,
			&currentTempl,
		); err != nil {
			return nil, err
		}

		if restoreTempl.Status != pgtype.Null {
			d.RestoreTemplate = restoreTempl.String
		}
		if currentTempl.Status != pgtype.Null {
			d.CurrentTemplate = currentTempl.String
		}
		data = append(data, &d)
	}
	return data, nil
}

func (db *Store) UpdateAvailableDataAfterRestoreFile(ctx context.Context, id int) error {
	_, err := db.Exec(ctx,
		"UPDATE"+db.pgEntity("table", "available_data")+"SET restored_at=now() WHERE id=$1", id)

	return err
}

func (db *Store) RestoreData(ctx context.Context, data *datastructs.RestoreData, tmpFile io.Reader) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if data.SingleTable {

		if data.RestoreTemplate == "" || data.RestoreTemplate != data.CurrentTemplate {
			return errors.New("no template for creating a table")
		}

		query := fmt.Sprintf(`SELECT public.f_clone_table_structure('%s.%s', '%s'::regclass);`, data.SchemaName, data.TableName, data.RestoreTemplate)
		if _, err := tx.Exec(ctx, query); err != nil {
			return fmt.Errorf("create table: %w", err)
		}

	}

	ctxCopy, cancel := context.WithTimeout(ctx, time.Duration(5*time.Minute))
	defer cancel()

	query := fmt.Sprintf(`COPY %s.%s FROM STDIN WITH CSV NULL 'NULL' DELIMITER ';' HEADER ;`, data.SchemaName, data.TableName)
	tag, err := tx.Conn().PgConn().CopyFrom(ctxCopy, tmpFile, query)
	if err != nil {
		return fmt.Errorf("copy from file or stdin to table:%s :%w", data.TableName, err)
	}
	if tag.RowsAffected() != data.ContentRows {
		return errors.New("the number of restored rows does not match the declared")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
