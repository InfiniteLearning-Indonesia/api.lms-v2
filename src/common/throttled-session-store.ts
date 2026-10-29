import connectPgSimple from 'connect-pg-simple';
import session, { type SessionData } from 'express-session';

const PgStore = connectPgSimple(session);

export interface ThrottledSessionStoreOptions
  extends connectPgSimple.PGStoreOptions {
  touchIntervalMs?: number;
  maxTrackedSessions?: number;
}

export class ThrottledSessionStore extends PgStore {
  private readonly lastTouches = new Map<string, number>();
  private readonly touchIntervalMs: number;
  private readonly maxTrackedSessions: number;

  constructor({
    touchIntervalMs = 300000,
    maxTrackedSessions = 5000,
    ...options
  }: ThrottledSessionStoreOptions = {}) {
    super(options);
    this.touchIntervalMs = touchIntervalMs;
    this.maxTrackedSessions = maxTrackedSessions;
  }

  override touch(
    sid: string,
    session: SessionData,
    callback?: (err?: Error | null) => void,
  ): void {
    const now = Date.now();
    const lastTouch = this.lastTouches.get(sid);
    if (lastTouch !== undefined && now - lastTouch < this.touchIntervalMs) {
      callback?.(null);
      return;
    }

    super.touch(sid, session, callback);

    if (this.maxTrackedSessions <= 0) return;
    this.lastTouches.delete(sid);
    if (this.lastTouches.size >= this.maxTrackedSessions) {
      const oldest = this.lastTouches.keys().next();
      if (!oldest.done) this.lastTouches.delete(oldest.value);
    }
    this.lastTouches.set(sid, now);
  }

  override destroy(sid: string, callback?: (err?: Error | null) => void): void {
    this.lastTouches.delete(sid);
    super.destroy(sid, callback);
  }
}
