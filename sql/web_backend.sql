DROP SCHEMA IF EXISTS web_backend__archive_manager CASCADE;

CREATE SCHEMA web_backend__archive_manager;

CREATE TABLE web_backend__archive_manager.t_config_table_gui (
	id serial4 NOT NULL,
	capt_area varchar(50) NOT NULL,
	conf_id int4 NOT NULL,
	caption varchar(100) NOT NULL,
	descrition varchar(500) NULL,
	gui_order int2 NOT NULL DEFAULT 0,
	CONSTRAINT t_config_table_gui_conf_id_caption_key UNIQUE (conf_id, caption),
	CONSTRAINT t_config_table_gui_pkey PRIMARY KEY (id)
);

INSERT INTO web_backend__archive_manager.t_config_table_gui (capt_area,conf_id,caption,descrition,gui_order) VALUES
	 ('Billing',2,'Invoice CDRs','Data related to Carriers Invoice',1),
	 ('Billing',5,'Reconciliation CDRs','Data related to Suppliers Reconcialiation Invoice',1),
	 ('Traffic Statistics',6,'Regular Statistics','RT tables',10),
	 ('Customer Rates',8,'Offered Customer Rates','Rates related to a customer',3),
	 ('Supplier Rates',4,'Supplier Rate History','Supplier rates in LCR format',2),
	 ('Supplier Rates',9,'Supplier Rate History','Original supplier rates',2);

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_set_enabled_backup_entity(in_id integer, in_enabled boolean)
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.config_table_list SET enabled = in_enabled WHERE id = in_id;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_stop_manager()
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.control SET stop_manager = true;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_edit_config_manager(w_backup integer, w_restore integer, in_tmp_folder text, OUT status varchar, OUT message varchar)
RETURNS RECORD
LANGUAGE plpgsql AS $$
BEGIN
	status	:= 'OK';
	message	:= '';
	--TODO: потом проверку на нулевые значения
	UPDATE archive_manager.config_manager SET workers_backup = w_backup, workers_restore = w_restore, tmp_folder = in_tmp_folder;
	GET DIAGNOSTICS message = ROW_COUNT;
	IF message <> '0' THEN 
		message := 'Editing data updated successfully !';
		UPDATE archive_manager.control SET reload_config = true;
	ELSE
		message	:= 'Editing data has not been updated !';
		status := 'FAILED';
	END IF;	
END;
$$;


CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_save_edit_backup_entity(
	in_id integer, 
	in_schemaname varchar,
	in_tblname_pattern varchar,
	in_arch_data_after integer,
	in_arch_interval_name varchar,
	in_keep_arch_days integer,
	in_entity varchar,
	in_condition_fmt text,
	in_date_clmn varchar,
	in_tblname_date_fmt_sbstr varchar,
	in_tblname_date_fmt_todate varchar,
	in_restore_template varchar,
	in_check_interval varchar,
	in_check_interval_id integer[],
	in_check_time_start time,
	in_check_time_stop time,
	OUT status varchar, 
	OUT message varchar)
RETURNS RECORD
LANGUAGE plpgsql AS $$
DECLARE
	_schemaname varchar;
	_tblname_pattern varchar;
	_arch_data_after integer;
	_arch_interval_name varchar;
	_entity varchar;
	_condition_fmt text;
	_date_clmn varchar;
	_check_interval varchar;
	_check_interval_id integer[];
	_check_time_start time without time zone;
	_check_time_stop time without time zone;
BEGIN
	status	:= 'OK';
	message	:= '';

	SELECT schemaname,tblname_pattern,arch_data_after,arch_interval_name,entity,condition_fmt,date_clmn,
			check_interval,check_interval_id,check_time_start,check_time_stop
	FROM archive_manager.config_table_list 
	INTO _schemaname,_tblname_pattern,_arch_data_after,_arch_interval_name,_entity,_condition_fmt,_date_clmn,
			_check_interval,_check_interval_id,_check_time_start,_check_time_stop
	WHERE id = in_id;
	
	IF in_schemaname = '' THEN 
		in_schemaname := _schemaname;
	END IF;
	IF in_tblname_pattern = '' THEN 
		in_tblname_pattern := _tblname_pattern;
	END IF;
	IF in_arch_data_after = 0 THEN 
		in_arch_data_after := _arch_data_after;
	END IF;
	IF in_arch_interval_name = '' OR in_arch_interval_name != 'DAY' OR in_arch_interval_name != 'MONTH' OR in_arch_interval_name != 'YEAR' THEN 
		in_arch_interval_name := _arch_interval_name;
	END IF;
	IF in_entity = '' OR in_entity !='record' OR in_entity != 'table' THEN 
		in_entity := _entity;
	END IF;
	IF in_condition_fmt = '' THEN 
		in_condition_fmt := _condition_fmt;
	END IF;
	IF in_date_clmn = '' THEN 
		in_date_clmn := _date_clmn;
	END IF;
	IF in_check_interval = '' OR in_check_interval != 'WEEK' OR in_check_interval != 'MONTH' THEN 
		in_check_interval := _check_interval;
	END IF;
	IF array_length(in_check_interval_id, 1) = 0 THEN 
		in_check_interval_id := _check_interval_id;
	END IF;

	UPDATE archive_manager.config_table_list SET schemaname=in_schemaname,tblname_pattern=in_tblname_pattern,arch_data_after=in_arch_data_after,
	arch_interval_name=in_arch_interval_name,keep_arch_days=in_keep_arch_days,entity=in_entity,condition_fmt=in_condition_fmt,date_clmn=in_date_clmn,
	tblname_date_fmt_sbstr=in_tblname_date_fmt_sbstr,tblname_date_fmt_todate=in_tblname_date_fmt_todate,restore_template=in_restore_template,
	enabled=false,check_interval=in_check_interval,check_interval_id=in_check_interval_id,check_time_start=in_check_time_start,
	check_time_stop=in_check_time_stop,reload=true
	WHERE id = in_id;
	GET DIAGNOSTICS message = ROW_COUNT;
	IF message <> '0' THEN 	message	:= 'Editing data updated successfully !';
	ELSE
    message	:= 'Editing data has not been updated !';
    status := 'FAILED';
	END IF;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_start_recovery(in_type_archive integer, in_date date)
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.control SET start_recovery = true, type_archive = in_type_archive, this_date = in_date;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_schema_dropdown_filter()
RETURNS TABLE(
	id integer,
	schemaname varchar
)
LANGUAGE plpgsql AS $$
BEGIN RETURN QUERY
	SELECT ctl.id, ctl.schemaname FROM archive_manager.config_table_list ctl;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_show_config_table_list(in_id integer DEFAULT NULL::integer) 
RETURNS TABLE(
	id integer,
	schemaname varchar,
	tblname_pattern varchar,
	arch_data_after integer,
	arch_interval_name varchar,
	keep_arch_days integer,
	entity varchar ,
	condition_fmt text,
	date_clmn varchar,
	tblname_date_fmt_sbstr varchar,
	tblname_date_fmt_todate varchar,
	restore_template varchar,
	enabled bool,
	check_interval varchar,
	check_interval_id integer[],
	check_time_start time without time zone,
	check_time_stop time without time zone
)
LANGUAGE plpgsql AS $$
BEGIN 
	IF in_id IS NULL THEN
		RETURN QUERY
			SELECT ctl.id, ctl.schemaname, ctl.tblname_pattern, ctl.arch_data_after, ctl.arch_interval_name, ctl.keep_arch_days, ctl.entity, ctl.condition_fmt,
			ctl.date_clmn, ctl.tblname_date_fmt_sbstr, ctl.tblname_date_fmt_todate, ctl.restore_template, ctl.enabled, ctl.check_interval, ctl.check_interval_id,
			ctl.check_time_start, ctl.check_time_stop
			FROM archive_manager.config_table_list ctl;
	ELSE 
		RETURN QUERY
			SELECT ctl.id, ctl.schemaname, ctl.tblname_pattern, ctl.arch_data_after, ctl.arch_interval_name, ctl.keep_arch_days, ctl.entity, ctl.condition_fmt,
			ctl.date_clmn, ctl.tblname_date_fmt_sbstr, ctl.tblname_date_fmt_todate, ctl.restore_template, ctl.enabled, ctl.check_interval, ctl.check_interval_id,
			ctl.check_time_start, ctl.check_time_stop
			FROM archive_manager.config_table_list ctl
			WHERE ctl.id = in_id;
	END IF;
END;
$$;


CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_delete_config_table(in_id integer, OUT status varchar, OUT message varchar)
RETURNS RECORD
LANGUAGE plpgsql AS $$
BEGIN 
	status	:= 'OK';
	message	:= '';
	DELETE FROM archive_manager.config_table_list ctl WHERE ctl.id = in_id;
	IF message <> '0' THEN 
		message := 'Deleted successfully !';
	ELSE
		message	:= 'Couldn`t delete the entry  !';
		status := 'FAILED';
	END IF;
END;
$$;


CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_save_new_backup_entity(
	in_schemaname varchar,
	in_tblname_pattern varchar,
	in_arch_data_after integer,
	in_arch_interval_name varchar,
	in_keep_arch_days integer,
	in_entity varchar,
	in_condition_fmt text,
	in_date_clmn varchar,
	in_tblname_date_fmt_sbstr varchar,
	in_tblname_date_fmt_todate varchar,
	in_restore_template varchar,
	in_check_interval varchar,
	in_check_interval_id integer[],
	in_check_time_start time,
	in_check_time_stop time,
	OUT status varchar, 
	OUT message varchar)
RETURNS RECORD
LANGUAGE plpgsql AS $$
BEGIN
	status	:= 'OK';
	message	:= '';

	INSERT INTO archive_manager.config_table_list (schemaname,tblname_pattern,arch_data_after,arch_interval_name,keep_arch_days,entity,condition_fmt,date_clmn,
	tblname_date_fmt_sbstr,tblname_date_fmt_todate,restore_template,check_interval,check_interval_id,check_time_start,check_time_stop)
	VALUES(in_schemaname,in_tblname_pattern,in_arch_data_after,in_arch_interval_name,in_keep_arch_days,in_entity,in_condition_fmt,in_date_clmn,
	in_tblname_date_fmt_sbstr,in_tblname_date_fmt_todate,in_restore_template,in_check_interval,in_check_interval_id,in_check_time_start,in_check_time_stop);
	GET DIAGNOSTICS message = ROW_COUNT;
	IF message <> '0' THEN 	message	:= 'Data added successfully  !';
	ELSE
    message	:= 'Couldn`t add new data  !';
    status := 'FAILED';
	END IF;
END;
$$;


CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_show_arch_available_data(in_id integer DEFAULT NULL::integer)
RETURNS TABLE (
	id integer,
	schemaname varchar,
	tblname varchar,
	blsingle_tbl_arch bool,
	content_min_date date,
	content_max_date date,
	content_rows integer,
	archived_at timestamptz,
	restored_at timestamptz
)
LANGUAGE plpgsql AS $$
BEGIN
	IF in_id IS NULL THEN
		RETURN QUERY
			SELECT * FROM archive_manager.arch_available_data;
	ELSE
		RETURN QUERY
			SELECT aad.* FROM archive_manager.arch_available_data aad 
		    JOIN archive_manager.config_table_list ctl  ON aad.schemaname = ctl.schemaname 
		    WHERE ctl.id = in_id;
	END IF;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_show_archive_statistics()
RETURNS TABLE (
	file_name varchar,
	file_size varchar,
	data_size varchar,
	available_data integer,	
	content_min_date date,
	content_max_date date,
	content_rows integer,
	created_at timestamptz,
	deleted_at timestamptz,
	comment text,
	restore_template varchar
)
LANGUAGE plpgsql AS $$
BEGIN
	RETURN QUERY
		SELECT * FROM archive_manager.archive_statistics;

END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_show_storage_settings(in_id integer)
RETURNS TABLE (
	data_id integer,
	storage_name varchar,
	storage_path varchar,
	use_sftp bool,
	storage_period integer,
	auto_cleaning bool,
	host varchar,
	port varchar,
	username varchar,
	pass varchar,
	auth_method varchar,
	time_out integer,
	privat_key varchar
)
LANGUAGE plpgsql AS $$
BEGIN 
	RETURN QUERY
			SELECT ss.*, csf.host, csf.port, csf.username, csf.pass, csf.auth_method, csf.time_out, csf.privat_key
			FROM archive_manager.storage_settings ss 
			LEFT OUTER JOIN archive_manager.config_sftp csf ON ss.storage_name = csf.storage_name
			WHERE ss.data_id = in_id;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_reload_config()
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.control SET reload_config = true;
END;
$$;


CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_show_manager_info(OUT message varchar, OUT state varchar, OUT status varchar,)
RETURNS RECORD
LANGUAGE plpgsql AS $$
DECLARE
	_state text;
BEGIN
	message	:= '';
	state	:= 'UNKNOWN SERVICE STATE!';
	status	:= 'Undefined';
	SELECT status_message, current_state INTO message, _state FROM archive_manager.control;
	CASE _state
		WHEN 'Service stopped...' THEN 
			status := 'Disabled';
			state  := 'Stopped';
		WHEN 'The service is running but the workers are not active' THEN
			status := 'Inactive';
			state  := 'Pending';
		WHEN 'The backup process is in progress...' THEN
			status := 'Active';
			state  := 'Running backup';
		WHEN 'The recovery process is in progress...' THEN
			status := 'Active';
			state  := 'Running recovery';
	END CASE;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_save_storage_settings(
	in_data_id integer, 
	in_storage_name varchar, 
	in_storage_path varchar, 
	in_use_sftp bool, 
	in_storage_period integer, 
	in_auto_cleaning bool,
	in_host varchar,
	in_port varchar,
	in_user_name varchar,
	in_pass varchar,
	in_auth_method varchar,
	in_time_out integer,
	in_privat_key varchar,
	OUT status varchar, OUT message varchar)
RETURNS RECORD
LANGUAGE plpgsql AS $$
BEGIN
	status	:= 'OK';
	message	:= '';
	INSERT INTO archive_manager.storage_settings VALUES (in_data_id,in_storage_name,in_storage_path,in_use_sftp,in_storage_period,in_auto_cleaning)
	ON CONFLICT (data_id) DO UPDATE
	SET storage_name=in_storage_name,storage_path=in_storage_path,use_sftp=in_use_sftp,storage_period=in_storage_period,auto_cleaning=in_auto_cleaning;
	GET DIAGNOSTICS message = ROW_COUNT;
	IF message <> '0' THEN 	message	:= 'Settings updated and saved successfully !';
	ELSE
    message	:= 'Couldn`t save settings !';
    status := 'FAILED';
	END IF;
	IF in_use_sftp = true THEN
		INSERT INTO archive_manager.config_sftp VALUES (in_storage_name,in_host,in_port,in_user_name,in_pass,in_auth_method,in_time_out,in_privat_key)
		ON CONFLICT (storage_name) DO UPDATE
		SET host=in_host,port=in_port,username=in_user_name,pass=in_pass,auth_method=in_auth_method,time_out=in_time_out,privat_key=in_privat_key;
		GET DIAGNOSTICS message = ROW_COUNT;
		IF message <> '0' THEN 	message	:= 'Settings saved successfully !';
		ELSE
		message	:= 'Couldn`t save settings !';
		status := 'FAILED';
		END IF;
	END IF;
END;
$$;

CREATE OR REPLACE FUNCTION web_backend__archive_manager.f_start_manual_backup(in_type_archive integer)
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.control SET start_manual_backup = true, type_archive = in_type_archive;
END;
$$;