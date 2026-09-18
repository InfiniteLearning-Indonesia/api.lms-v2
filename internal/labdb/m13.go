package labdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/platform/transaction"
)

const durableDeliverySchema = `CREATE TABLE v3ops.outbox_provider_receipts(
 event_id text NOT NULL REFERENCES v3.outbox(event_id),
 attempt_number integer NOT NULL CHECK(attempt_number>0),
 lease_id text NOT NULL CHECK(lease_id ~ '^[a-f0-9]{32}$'),
 provider_key text NOT NULL CHECK(provider_key ~ '^[A-Za-z0-9._:-]{1,160}$'),
 outcome text NOT NULL CHECK(outcome IN ('CONFIRMED','DUPLICATE')),
 reference_hash text NOT NULL CHECK(reference_hash ~ '^[a-f0-9]{64}$'),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(event_id,attempt_number));
CREATE INDEX outbox_provider_receipts_occurred_idx ON v3ops.outbox_provider_receipts(occurred_at,event_id);
CREATE INDEX outbox_provider_receipts_provider_idx ON v3ops.outbox_provider_receipts(provider_key,occurred_at);
CREATE FUNCTION v3ops.guard_provider_receipt_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PROVIDER_RECEIPT_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3ops.guard_provider_receipt_immutable() FROM PUBLIC;
CREATE TRIGGER provider_receipt_immutable BEFORE UPDATE ON v3ops.outbox_provider_receipts FOR EACH ROW EXECUTE FUNCTION v3ops.guard_provider_receipt_immutable();`

const notificationSafetySchema = `CREATE TABLE v3.notification_preference_events(
 user_id text NOT NULL REFERENCES v3.users(id),channel text NOT NULL CHECK(channel IN ('EMAIL')),
 version bigint NOT NULL CHECK(version>0),enabled boolean NOT NULL,actor_id text NOT NULL REFERENCES v3.users(id),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(user_id,channel,version));
CREATE INDEX notification_preference_events_current_idx ON v3.notification_preference_events(user_id,channel,version DESC);
CREATE INDEX notification_preference_events_actor_idx ON v3.notification_preference_events(actor_id,occurred_at);
CREATE TABLE v3.notification_commands(actor_id text NOT NULL REFERENCES v3.users(id),key text NOT NULL,
 request_hash text NOT NULL CHECK(request_hash ~ '^[a-f0-9]{64}$'),response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(actor_id,key));
CREATE TABLE v3.notification_deliveries(
 id text PRIMARY KEY,event_id text NOT NULL REFERENCES v3.outbox(event_id),user_id text NOT NULL REFERENCES v3.users(id),
 channel text NOT NULL CHECK(channel IN ('EMAIL')),template_key text NOT NULL CHECK(template_key ~ '^[a-z][a-z0-9_.-]{0,63}$'),
 source text NOT NULL CHECK(source IN ('OPERATIONAL','HISTORICAL_IMPORT')),
 status text NOT NULL CHECK(status IN ('QUEUED','SUPPRESSED')),
 suppression_code text CHECK(suppression_code IN ('PREFERENCE_DISABLED','PROVIDER_DISABLED','HISTORICAL_IMPORT')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(event_id,user_id,channel,template_key),
 CHECK((status='QUEUED' AND suppression_code IS NULL AND source='OPERATIONAL') OR (status='SUPPRESSED' AND suppression_code IS NOT NULL)));
CREATE INDEX notification_deliveries_user_idx ON v3.notification_deliveries(user_id,created_at,id);
CREATE INDEX notification_deliveries_event_fk_idx ON v3.notification_deliveries(event_id);
CREATE INDEX notification_deliveries_pending_idx ON v3.notification_deliveries(created_at,id) WHERE status='QUEUED';
CREATE TABLE v3ops.provider_binding_events(
 channel text NOT NULL CHECK(channel IN ('EMAIL')),version bigint NOT NULL CHECK(version>0),
 adapter text NOT NULL CHECK(adapter IN ('REHEARSAL','SMTP','HOLIDAY_API')),
 secret_ref text NOT NULL CHECK(secret_ref ~ '^secretref://[A-Za-z0-9._/-]{1,200}$'),enabled boolean NOT NULL,
 actor_id text NOT NULL REFERENCES v3.users(id),occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(channel,version));
CREATE INDEX provider_binding_events_current_idx ON v3ops.provider_binding_events(channel,version DESC);
CREATE FUNCTION v3.guard_notification_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='NOTIFICATION_HISTORY_IMMUTABLE'; END $$;
REVOKE ALL ON FUNCTION v3.guard_notification_immutable() FROM PUBLIC;
CREATE TRIGGER notification_preference_immutable BEFORE UPDATE ON v3.notification_preference_events FOR EACH ROW EXECUTE FUNCTION v3.guard_notification_immutable();
CREATE TRIGGER notification_command_immutable BEFORE UPDATE ON v3.notification_commands FOR EACH ROW EXECUTE FUNCTION v3.guard_notification_immutable();
CREATE TRIGGER notification_delivery_immutable BEFORE UPDATE ON v3.notification_deliveries FOR EACH ROW EXECUTE FUNCTION v3.guard_notification_immutable();
CREATE TRIGGER provider_binding_immutable BEFORE UPDATE ON v3ops.provider_binding_events FOR EACH ROW EXECUTE FUNCTION v3.guard_notification_immutable();
CREATE FUNCTION v3.guard_notification_delivery_insert() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,v3,v3ops AS $$ DECLARE pref boolean; provider boolean; BEGIN
 IF NEW.source='HISTORICAL_IMPORT' THEN
  IF NEW.status<>'SUPPRESSED' OR NEW.suppression_code<>'HISTORICAL_IMPORT' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='IMPORT_OUTBOUND_REJECTED'; END IF;
  RETURN NEW;
 END IF;
 SELECT enabled INTO pref FROM v3.notification_preference_events WHERE user_id=NEW.user_id AND channel=NEW.channel ORDER BY version DESC LIMIT 1;
 SELECT enabled INTO provider FROM v3ops.provider_binding_events WHERE channel=NEW.channel ORDER BY version DESC LIMIT 1;
 IF coalesce(pref,true)=false THEN
  IF NEW.status<>'SUPPRESSED' OR NEW.suppression_code<>'PREFERENCE_DISABLED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PREFERENCE_BYPASS_REJECTED'; END IF;
 ELSIF coalesce(provider,false)=false THEN
  IF NEW.status<>'SUPPRESSED' OR NEW.suppression_code<>'PROVIDER_DISABLED' THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='PROVIDER_BYPASS_REJECTED'; END IF;
 ELSIF NEW.status<>'QUEUED' OR NEW.suppression_code IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='DELIVERY_STATE_REJECTED';
 END IF;
 RETURN NEW;
END$$;
REVOKE ALL ON FUNCTION v3.guard_notification_delivery_insert() FROM PUBLIC;
CREATE TRIGGER notification_delivery_insert_guard BEFORE INSERT ON v3.notification_deliveries FOR EACH ROW EXECUTE FUNCTION v3.guard_notification_delivery_insert();`

func DurableDeliveryChecksum() string    { return Digest([]byte(durableDeliverySchema)) }
func NotificationSafetyChecksum() string { return Digest([]byte(notificationSafetySchema)) }

func installM13(ctx context.Context, pool *pgxpool.Pool, checkpoint, archive, version, ddl, dependency, expected string) error {
	if pool == nil || VerifyInstallmentProof(checkpoint, archive) != nil {
		return errors.New("LAB_BACKUP_GATE_REJECTED")
	}
	c := pool.Config().ConnConfig
	if c.Host != "127.0.0.1" || c.Database != "qa_cp05" || c.User != "qa_fixture" || c.TLSConfig != nil || Identity(ctx, pool) != nil {
		return errors.New("LAB_TARGET_REQUIRED")
	}
	return transaction.Within(ctx, pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(810913)").Scan(&locked); err != nil || !locked {
			return errors.New("MIGRATION_BUSY")
		}
		var checksum string
		if tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", dependency).Scan(&checksum) != nil || checksum != expected {
			return errors.New("MIGRATION_DEPENDENCY_REQUIRED")
		}
		err := tx.QueryRow(ctx, "SELECT checksum FROM v3ops.migrations WHERE version=$1", version).Scan(&checksum)
		if err == nil {
			if checksum != Digest([]byte(ddl)) {
				return errors.New("MIGRATION_CHECKSUM_CONFLICT")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, ddl, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3ops.migrations(version,checksum,backup_hash) VALUES($1,$2,$3)", version, Digest([]byte(ddl)), proofArchiveHash(checkpoint))
		return err
	})
}

func InstallDurableDelivery(ctx context.Context, pool *pgxpool.Pool) error {
	return installM13(ctx, pool, "m13-cp01", "pre_m13_cp01.dump", "m13-cp01-1", durableDeliverySchema, "m12-cp03-1", LogbookReviewChecksum())
}

func InstallNotificationSafety(ctx context.Context, pool *pgxpool.Pool) error {
	return installM13(ctx, pool, "m13-cp02", "pre_m13_cp02.dump", "m13-cp02-1", notificationSafetySchema, "m13-cp01-1", DurableDeliveryChecksum())
}
