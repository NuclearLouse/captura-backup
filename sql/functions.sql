-- internal by Alex function
CREATE OR REPLACE FUNCTION archive_manager.f_check_tbls_to_archive(i_id integer DEFAULT NULL::integer, i_entity character varying DEFAULT NULL::character varying, OUT o_cnt integer)
 RETURNS integer
 LANGUAGE plpgsql
AS $$
DECLARE
	qry		text;
	rec 	record;
	icnt	integer;
BEGIN
	o_cnt	:= -1;
	IF i_id IS NULL AND i_entity IS NULL THEN 
		TRUNCATE TABLE archive_manager.pr_arch_tbls;
		SELECT f.o_cnt INTO o_cnt FROM archive_manager.f_check_tbls_to_archive(null, 'record') f;
		SELECT f.o_cnt INTO icnt FROM archive_manager.f_check_tbls_to_archive(null, 'table') f;
		o_cnt	:= o_cnt +icnt;
		RETURN;
	END IF;	
	
	IF i_entity IS NULL THEN 
		SELECT entity INTO i_entity FROM archive_manager.config_table_list WHERE id = i_id ;
	END IF;
	IF i_entity IS NULL THEN RETURN; END IF;

	DROP TABLE IF EXISTS tmp_archid;
	IF i_id IS NOT NULL THEN
		CREATE TEMP TABLE tmp_archid AS SELECT i_id AS id;
		-- Clear current data for the category
		DELETE FROM archive_manager.pr_arch_tbls WHERE tid = i_id;
	ELSE
		CREATE TEMP TABLE tmp_archid AS SELECT id FROM archive_manager.config_table_list ;
		-- Clear current data for all categories for the entity
		IF i_entity IS NOT NULL THEN
			DELETE FROM archive_manager.pr_arch_tbls WHERE entity = i_entity;
		END IF;
	END IF;
	
	IF i_entity = 'record' THEN
		/* check arch data inside table using condition provided */
		INSERT INTO archive_manager.pr_arch_tbls (tblname, tid, entity, date_clmn, condtion_fmt, rm_interval, do_backup)
		SELECT 	
				format('%s.%I', pt.schemaname, pt.tablename) tbl_full,
				id,
				entity,
				date_clmn,
				condition_fmt,
				arch_data_after||' '||arch_interval_name as rmv_int,
				keep_arch_days >= 0 as do_backup
			FROM archive_manager.config_table_list ct
			JOIN pg_tables pt ON pt.schemaname = ct.schemaname AND pt.tablename ~ ct.tblname_pattern		
			WHERE entity = 'record' AND condition_fmt IS NOT NULL AND id IN (SELECT id FROM tmp_archid)
				/*Check Time of the Day*/
				--AND archive_manager.f_check_interval_isnow(check_interval, check_interval_id, check_time_start, check_time_stop)			
				/*
				TODO: Add check of how long ago last archiving of a table was done
				*/
			ORDER BY 1
			ON CONFLICT (tblname) DO UPDATE SET
				date_clmn		= EXCLUDED.date_clmn, 
				condtion_fmt	= EXCLUDED.condtion_fmt,
				rm_interval		= EXCLUDED.rm_interval,
				do_backup		= EXCLUDED.do_backup;
			
			GET DIAGNOSTICS o_cnt = ROW_COUNT;
			RAISE NOTICE 'Tables collected for "record" entity: %', o_cnt;
	ELSEIF i_entity = 'table' THEN 
		/* arch date is encoded into table name */
		INSERT INTO archive_manager.pr_arch_tbls (tblname, tid, entity, date_clmn, condtion_fmt, rm_interval, do_backup)
		SELECT 	
				format('%s.%I', pt.schemaname, pt.tablename) tbl_full,
				id, 
				entity,
				date_clmn,
				condition_fmt,				
				arch_data_after||' '||arch_interval_name as rmv_int,
				keep_arch_days >= 0 as do_backup
			FROM archive_manager.config_table_list ct
			JOIN pg_tables pt ON pt.schemaname = ct.schemaname AND pt.tablename ~ ct.tblname_pattern,
			LATERAL (SELECT TO_DATE(SUBSTRING(tablename, tblname_date_fmt_sbstr), tblname_date_fmt_todate) tbl_date,
					 /*this is to remove whole month data*/
					(CASE WHEN tblname_date_fmt_todate !~* 'D' THEN date_trunc('MONTH',current_date)::DATE
						ELSE current_date END - (arch_data_after||' '||arch_interval_name)::INTERVAL)::DATE ref_date) v
			WHERE ct.entity = 'table' AND condition_fmt IS NULL 
				AND tbl_date < ref_date
				AND id IN (SELECT id FROM tmp_archid)
				/*Check Time of the Day*/
				--AND archive_manager.f_check_interval_isnow(check_interval, check_interval_id, check_time_start, check_time_stop)	
			ON CONFLICT (tblname) DO UPDATE SET
				date_clmn		= EXCLUDED.date_clmn, 
				condtion_fmt	= EXCLUDED.condtion_fmt,
				rm_interval		= EXCLUDED.rm_interval,
				do_backup		= EXCLUDED.do_backup;			
		
		GET DIAGNOSTICS o_cnt = ROW_COUNT;
		RAISE NOTICE 'Tables collected for regular tables: %', o_cnt;
		
		/* get arch table from the query in config table */
		FOR rec IN (SELECT * FROM archive_manager.config_table_list 
					WHERE entity = 'table' AND condition_fmt IS NOT NULL AND id IN (SELECT id FROM tmp_archid)
				   		/*Check Time of the Day*/
						--AND archive_manager.f_check_interval_isnow(check_interval, check_interval_id, check_time_start, check_time_stop)
				   )
		LOOP
		EXECUTE 'WITH tl AS ('||rec.condition_fmt||')
			INSERT INTO archive_manager.pr_arch_tbls (tblname, tid, entity, date_clmn, condtion_fmt, rm_interval, do_backup)
			SELECT 	
					format(''%s.%I'', pt.schemaname, pt.tablename) tbl_full,
					id, 
					entity,
					date_clmn, 
					null::text condition_fmt,
					$2 as rmv_int,
					keep_arch_days >= 0 as do_backup
				FROM tl
				JOIN archive_manager.config_table_list ct ON id = $3
				JOIN pg_tables pt ON pt.schemaname = ct.schemaname AND pt.tablename = tl.tblname
				ORDER BY 1
				ON CONFLICT (tblname) DO UPDATE SET
					date_clmn		= EXCLUDED.date_clmn, 
					condtion_fmt	= EXCLUDED.condtion_fmt,
					rm_interval		= EXCLUDED.rm_interval,
					do_backup		= EXCLUDED.do_backup;	'
			USING (current_date - (rec.arch_data_after||' '||rec.arch_interval_name)::INTERVAL)::DATE, 
				rec.arch_data_after||' '||rec.arch_interval_name, rec.id;	
		
			GET DIAGNOSTICS icnt = ROW_COUNT;
			RAISE NOTICE 'Tables collected for "%" tables: %', rec.tblname_pattern, icnt;	
			o_cnt	:= o_cnt +icnt;
		END LOOP;
	END IF;
END
$$
;

-- internal by Alex function
CREATE OR REPLACE FUNCTION archive_manager.f_check_interval_isnow(i_interval character varying, i_intervalids integer[], i_startteime time without time zone, i_stoptime time without time zone)
 RETURNS boolean
 LANGUAGE plpgsql
AS $$
DECLARE
	idayid		integer;
	iprevdayid	integer;
	o_intisnow	boolean;
BEGIN
	CASE i_interval 
		WHEN 'MONTH' THEN	
			idayid		:= EXTRACT(DAY FROM now());
			iprevdayid	:= EXTRACT(DAY FROM CURRENT_DATE -1);
			-- replace -1 with the current month last day
			i_intervalids	:= array_replace(i_intervalids, -1, 
											EXTRACT(DAY FROM date_trunc('MONTH', CURRENT_DATE) + INTERVAL '1 MONTH' - INTERVAL '1 DAY')::int);
		ELSE				
			idayid		:= EXTRACT(ISODOW FROM now());
			iprevdayid	:= EXTRACT(ISODOW FROM CURRENT_DATE -1);
	END CASE;
	
	IF i_startteime <= i_stoptime THEN
		/*start and stop are in the same date*/
		o_intisnow	:= CURRENT_TIME BETWEEN i_startteime AND i_stoptime AND idayid = ANY(i_intervalids);
	ELSEIF CURRENT_TIME >= i_startteime THEN
		/*start and stop are in different dates: Check start of the period until midnight*/
		o_intisnow	:= idayid = ANY(i_intervalids);
	ELSEIF CURRENT_TIME <= i_stoptime THEN
		/*start and stop are in different dates: Check end of period - this is always in the next date. Thus we are checking PrevDayID*/
		o_intisnow	:= iprevdayid = ANY(i_intervalids);
	ELSE	
		o_intisnow	:= FALSE;
	END IF;
	
	RETURN o_intisnow;
END;
$$
;

-- state_and_message
CREATE OR REPLACE FUNCTION archive_manager.f_set_state_and_message(s_state text, s_message text)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE archive_manager.control SET message = s_message, current_state = s_state;
END;
$$;

-- reset_state_control
CREATE OR REPLACE FUNCTION archive_manager.f_reset_state_control()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE archive_manager.control SET 
    stop_manager = FALSE, reload_config = FALSE, start_recovery = FALSE, start_manual_backup = FALSE, restore_to_this_date = NULL, type_archive = NULL;
END;
$$;

-- datas_to_archive
CREATE OR REPLACE FUNCTION archive_manager.f_datas_to_archive(in_data_id integer)
RETURNS TABLE (
	tblname varchar,
	tid integer,
	entity varchar,
	date_clmn varchar,
	condtion_fmt varchar,
	rm_interval varchar,
	do_backup bool,
	restore_template varchar,
	keep_restore_days int4
)
LANGUAGE plpgsql
AS $$ 
BEGIN 
	PERFORM * FROM archive_manager.f_check_tbls_to_archive(in_data_id);
	RETURN QUERY
	SELECT pat.*, ctl.restore_template, ctl.keep_restore_days FROM archive_manager.pr_arch_tbls pat 
	JOIN archive_manager.config_table_list ctl ON pat.tid = ctl.id
	WHERE pat.tid = in_data_id;
END;
$$;

CREATE OR REPLACE FUNCTION archive_manager.f_add_arch_available_data(
	i_data_id int4,
	s_schema_name varchar, 
	s_table_name varchar, 
	bl_single_table boolean, 
	s_file_name varchar,
	d_content_date date,
	i_content_rows int4,
	t_archived_at timestamptz,
	s_restore_template varchar
	)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN 
	INSERT INTO archive_manager.arch_available_data (data_id,schemaname,tblname,blsingle_tbl_arch,file_name,content_date,content_rows,archived_at,restore_template)
	VALUES (i_data_id,s_schema_name,s_table_name,bl_single_table,s_file_name,d_content_date,i_content_rows,t_archived_at,s_restore_template)
	ON CONFLICT ON CONSTRAINT uniq_arch_available_data DO UPDATE SET content_rows=i_content_rows, archived_at=t_archived_at,restore_template=s_restore_template;
	
END;
$$;

-- files_for_restore
CREATE OR REPLACE FUNCTION archive_manager.f_files_for_restore(i_data_id integer, d_this_date date)
RETURNS TABLE (
	id integer,
	data_id integer,
	filename varchar,
	schemaname varchar,
	tblname varchar,
	tbl_entity boolean,
	content_date date,
	content_rows int4,
	restore_template varchar,
	current_template varchar
)
LANGUAGE plpgsql AS $$
BEGIN 
	IF i_data_id = 0 THEN
	RETURN QUERY
	    SELECT aad.id,aad.data_id,aad.file_name,aad.schemaname,aad.tblname,aad.blsingle_tbl_arch,aad.content_date,aad.content_rows,aad.restore_template,ctl.restore_template
		FROM archive_manager.arch_available_data aad
		JOIN archive_manager.config_table_list ctl ON aad.data_id = ctl.id
		WHERE aad.content_date <= d_this_date AND aad.deleted_at IS NULL;
	ELSE
	RETURN QUERY
		SELECT aad.id,aad.data_id,aad.file_name,aad.schemaname,aad.tblname,aad.blsingle_tbl_arch,aad.content_date,aad.content_rows,aad.restore_template,ctl.restore_template
		FROM archive_manager.arch_available_data aad
		JOIN archive_manager.config_table_list ctl ON aad.data_id = ctl.id
		WHERE aad.content_date <= d_this_date AND aad.deleted_at IS NULL 
		AND aad.data_id = i_data_id;
	END IF;
END;
$$;

CREATE OR REPLACE FUNCTION archive_manager.f_common_template_fields (in_schema text, restore_template text, current_template text, OUT insert_into_columns text, OUT select_from_columns text) 
RETURNS RECORD
LANGUAGE plpgsql AS $$
BEGIN 
	insert_into_columns := '';
	select_from_columns := '';
	SELECT string_agg(quote_ident(t1.column_name), ',' ORDER BY t1.ordinal_position),
	string_agg(quote_ident(t2.column_name), ',' ORDER BY t1.ordinal_position) 
	INTO insert_into_columns, select_from_columns
	FROM information_schema.columns t1
	JOIN information_schema.columns t2 ON t1.column_name = t2.column_name
	AND t1.table_schema = in_schema AND t2.table_name = restore_template  
	WHERE t1.table_schema = in_schema  AND t1.table_name   = current_template;
END;
$$;

CREATE OR REPLACE FUNCTION archive_manager.f_set_enabled_backup_entity(in_id integer, in_enabled boolean)
RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
	UPDATE archive_manager.config_table_list SET enabled = in_enabled WHERE id = in_id;
END;
$$;