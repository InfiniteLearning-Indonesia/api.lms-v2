package labdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
)

const meetingCalendarSchema = `CREATE TABLE v3.class_meetings(id text PRIMARY KEY,class_id text NOT NULL REFERENCES v3.classes(id),title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),local_date date NOT NULL,timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 64),starts_at timestamptz NOT NULL,ends_at timestamptz NOT NULL,created_by text NOT NULL REFERENCES v3.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),CHECK(ends_at>starts_at AND ends_at<=starts_at+interval '8 hours'),CHECK(local_date=(starts_at AT TIME ZONE timezone)::date),UNIQUE(id,class_id));
CREATE INDEX class_meetings_calendar_idx ON v3.class_meetings(class_id,local_date,starts_at,id);
CREATE INDEX class_meetings_actor_idx ON v3.class_meetings(created_by,created_at);
CREATE TABLE v3.meeting_exceptions(id text PRIMARY KEY,class_id text NOT NULL,meeting_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),kind text NOT NULL CHECK(kind IN ('CANCELLED','REINSTATED')),reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 240),actor_id text NOT NULL REFERENCES v3.users(id),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),FOREIGN KEY(meeting_id,class_id) REFERENCES v3.class_meetings(id,class_id),UNIQUE(meeting_id,revision));
CREATE INDEX meeting_exceptions_class_idx ON v3.meeting_exceptions(class_id,meeting_id,revision DESC);
CREATE INDEX meeting_exceptions_actor_idx ON v3.meeting_exceptions(actor_id,occurred_at);
CREATE TABLE v3.meeting_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE FUNCTION v3.guard_meeting_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='MEETING_HISTORY_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_meeting_immutable() FROM PUBLIC;
CREATE TRIGGER class_meetings_immutable BEFORE UPDATE ON v3.class_meetings FOR EACH ROW EXECUTE FUNCTION v3.guard_meeting_immutable();
CREATE TRIGGER meeting_exceptions_immutable BEFORE UPDATE ON v3.meeting_exceptions FOR EACH ROW EXECUTE FUNCTION v3.guard_meeting_immutable();
CREATE TRIGGER meeting_commands_immutable BEFORE UPDATE ON v3.meeting_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_meeting_immutable();
CREATE FUNCTION v3.guard_meeting_exception_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3 AS $$ DECLARE last_revision bigint; last_kind text; BEGIN
 PERFORM 1 FROM v3.class_meetings WHERE id=NEW.meeting_id AND class_id=NEW.class_id FOR UPDATE;
 SELECT revision,kind INTO last_revision,last_kind FROM v3.meeting_exceptions WHERE meeting_id=NEW.meeting_id ORDER BY revision DESC LIMIT 1;
 IF NEW.revision IS DISTINCT FROM COALESCE(last_revision,0)+1 OR (NEW.kind='CANCELLED' AND last_kind='CANCELLED') OR (NEW.kind='REINSTATED' AND last_kind IS DISTINCT FROM 'CANCELLED') THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='MEETING_EXCEPTION_TRANSITION_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_meeting_exception_insert() FROM PUBLIC;
CREATE TRIGGER meeting_exception_insert_guard BEFORE INSERT ON v3.meeting_exceptions FOR EACH ROW EXECUTE FUNCTION v3.guard_meeting_exception_insert();`

func MeetingCalendarChecksum() string { return Digest([]byte(meetingCalendarSchema)) }

// InstallMeetingCalendar is the fixed local-synthetic CP01 DDL only. The
// caller must provide a fresh dump and actual isolated restore receipt.
func InstallMeetingCalendar(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil || VerifyInstallmentProof("m11-cp01", "pre_m11_cp01.dump") != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810910)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var previous string
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m05-cp04-1'").Scan(&previous) != nil || previous != Digest([]byte(participationProvenanceSchema)) {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m06-cp02-1'").Scan(&previous) != nil || previous != Digest([]byte(privateFileDeliverySchema)) {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version='m11-cp01-1'").Scan(&previous)
		if err == nil {
			if previous != MeetingCalendarChecksum() {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, meetingCalendarSchema, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES('m11-cp01-1',$1,$2)", MeetingCalendarChecksum(), proofArchiveHash("m11-cp01"))
		return err
	})
}
