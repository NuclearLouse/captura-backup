DROP SCHEMA IF EXISTS archive_manager CASCADE;

CREATE SCHEMA archive_manager;

CREATE TABLE archive_manager.config_table_list (
	id serial NOT NULL,
	schemaname varchar(64) NOT NULL,
	tblname_pattern varchar(64) NOT NULL,
	arch_data_after int4 NULL,
	arch_interval_name varchar(50) NOT NULL DEFAULT 'DAY'::character varying,
	keep_arch_days int4 NOT NULL DEFAULT 0,
	entity varchar NOT NULL DEFAULT 'table'::character varying,
	condition_fmt text NULL,
	date_clmn varchar(64) NULL,
	tblname_date_fmt_sbstr varchar(25) NULL,
	tblname_date_fmt_todate varchar(25) NULL,
	restore_template varchar(64) NULL,
	keep_restore_days int4 NULL,
	CONSTRAINT config_table_list_id_key UNIQUE (id),
	CONSTRAINT config_table_list_pkey PRIMARY KEY (tblname_pattern, schemaname)
);

COMMENT ON COLUMN archive_manager.config_table_list.keep_arch_days IS 'Setting responsible for two parameters. If the value is <0, then the data is simply deleted from the database without being saved. If the value is >= 0, then before deleting the data from the database, they are saved. At the same time, this value serves as an indication of how many days to store the saved data. If 0, then store forever.';
COMMENT ON COLUMN archive_manager.config_table_list.arch_interval_name IS 'Available values: "DAY", "MONTH", "YEAR"';
COMMENT ON COLUMN archive_manager.config_table_list.entity IS 'Available values: "record", "table"';

INSERT INTO archive_manager.config_table_list (
	schemaname,
	tblname_pattern,
	arch_data_after,
	arch_interval_name,
	keep_arch_days,
	entity,
	condition_fmt,
	date_clmn,
	tblname_date_fmt_sbstr,
	tblname_date_fmt_todate,
	restore_template,
	keep_restore_days
	)
	VALUES
	 ('sales','^RouteVKN\d*$',6,'MONTH',366,'record','"Gueltig_bis" < $1','"Gueltig_bis"',NULL,NULL,NULL,14),
	 ('billdb_inv','^inv_\d*_\d*_cdr$',6,'MONTH',0,'table','SELECT format(''inv_%s_%s_cdr'', "CarrierID", "CarrierInvoiceID") tblname
	FROM billdb."Bill_Invoice" WHERE "InvoiceDate" < $1','"ST_Date"',NULL,NULL,'billdb_inv._template_inv_cdr',14),
	 ('logdb','^Log_(RT|SP)?\d{6}$',2,'YEAR',-1,'table',NULL,'"Date"','\d{6}','YYYYMM',NULL,NULL),
	 ('lcr_db','^RB_P_\d*_\d*$',6,'MONTH',-1,'record','"Gueltigbis" < $1','"Gueltigbis"',NULL,NULL,NULL,NULL),
	 ('billdb_rec','^rec_\d*_\d*_cdr$',6,'MONTH',0,'table','SELECT format(''rec_%s_%s_cdr'', "CarrierID", "InvoiceID") tblname
	FROM billdb."BSup_Invoice" WHERE "InvoiceDate" < $1','"ST_Date"',NULL,NULL,'billdb_rec._template_rec_cdr',14),
	 ('raterresult','^RT\d{8}$',6,'MONTH',0,'table',NULL,'"St_Date"','\d{8}','YYYYMMDD','raterresult._template_rt',14);

CREATE TABLE archive_manager.schedule_settings (
	data_id integer NOT NULL,
	enabled bool NOT NULL DEFAULT true,
	check_interval varchar(10) NOT NULL DEFAULT 'WEEK'::character varying,
	check_interval_days _int4 NOT NULL DEFAULT ARRAY[6, 7],
	work_start time NOT NULL DEFAULT '01:01:00'::time without time zone,
	CONSTRAINT pk_schedule_settings PRIMARY KEY (data_id),
	CONSTRAINT fk_schedule_settings FOREIGN KEY (data_id) REFERENCES archive_manager.config_table_list (id)
	ON DELETE CASCADE ON UPDATE CASCADE	
);

COMMENT ON COLUMN archive_manager.schedule_settings.check_interval IS 'Determines when the service should back up - on certain days of the week or days of the month';
COMMENT ON COLUMN archive_manager.schedule_settings.check_interval_days IS 'Specifies the days of the week or days of the month on which to run the backup';
COMMENT ON COLUMN archive_manager.schedule_settings.work_start IS 'Backup start time';

INSERT INTO archive_manager.schedule_settings (
	data_id,
	check_interval,
	check_interval_days,
	work_start
)
VALUES 
	(1,'WEEK','{7}','00:00:00'),
	(2,'WEEK','{7}','00:00:00'),
	(3,'MONTH','{1}','00:00:00'),
	(4,'MONTH','{1,15}','00:00:00'),
	(5,'WEEK','{7}','00:00:00'),
	(6,'WEEK','{7}','00:00:00');

CREATE TABLE archive_manager.arch_available_data (
	id serial NOT NULL,
	data_id int4 NOT NULL,
	schemaname varchar(64) NOT NULL,
	tblname varchar(64) NOT NULL,
	blsingle_tbl_arch bool NOT NULL,
	file_name varchar NOT NULL,
	content_date date NOT NULL,
	content_rows int4 NOT NULL,
	archived_at timestamptz NOT NULL DEFAULT now(),
	restored_at timestamptz NULL,
	deleted_at timestamptz NULL,
	restore_template varchar NULL,
	CONSTRAINT uniq_arch_available_data UNIQUE (schemaname, tblname, content_date),
	CONSTRAINT pk_arch_available_data PRIMARY KEY (id)
);

CREATE TABLE archive_manager.pr_arch_tbls (
	tblname varchar(130) NOT NULL,
	tid int4 NOT NULL,
	entity varchar(25) NULL,
	date_clmn varchar(64) NULL,
	condtion_fmt varchar(100) NULL,
	rm_interval varchar(25) NULL,
	do_backup bool NULL,
	CONSTRAINT pr_arch_tbls_pkey PRIMARY KEY (tblname)
);

CREATE TABLE archive_manager.control (
	stop_manager bool NOT NULL DEFAULT false,
	reload_config bool NOT NULL DEFAULT false,
	start_recovery bool NOT NULL DEFAULT false,
	restore_to_this_date date NULL,
	start_manual_backup bool NOT NULL DEFAULT false,
	type_archive integer NULL, --all = 0 id from config_table_list
	message text NULL,
	current_state text NOT NULL
);

INSERT INTO archive_manager.control (stop_manager,current_state) VALUES (true,'Service stopped...');
