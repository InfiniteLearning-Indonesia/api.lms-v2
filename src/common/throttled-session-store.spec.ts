import type connectPgSimple from 'connect-pg-simple';
import session, { type SessionData } from 'express-session';
import { ThrottledSessionStore } from './throttled-session-store.js';

const pgStorePrototype = Object.getPrototypeOf(
  ThrottledSessionStore.prototype,
) as connectPgSimple.PGStore;

describe('ThrottledSessionStore', () => {
  const sessionData: SessionData = { cookie: new session.Cookie() };
  let store: ThrottledSessionStore;
  let touch: jest.SpiedFunction<connectPgSimple.PGStore['touch']>;
  let destroy: jest.SpiedFunction<connectPgSimple.PGStore['destroy']>;
  let now: jest.SpiedFunction<typeof Date.now>;

  beforeEach(() => {
    now = jest.spyOn(Date, 'now').mockReturnValue(0);
    touch = jest
      .spyOn(pgStorePrototype, 'touch')
      .mockImplementation((_sid, _session, callback) => callback?.());
    destroy = jest
      .spyOn(pgStorePrototype, 'destroy')
      .mockImplementation((_sid, callback) => callback?.(null));
    store = new ThrottledSessionStore({ pruneSessionInterval: false });
  });

  afterEach(() => {
    store.close();
    jest.restoreAllMocks();
  });

  it('delegates the first touch and its callback to PgStore', () => {
    const callback = jest.fn();

    store.touch('sid-1', sessionData, callback);

    expect(touch).toHaveBeenCalledTimes(1);
    expect(touch).toHaveBeenCalledWith('sid-1', sessionData, callback);
    expect(callback).toHaveBeenCalledTimes(1);
  });

  it('skips database writes within the default interval and calls back with null', () => {
    store.touch('sid-1', sessionData);
    now.mockReturnValue(299999);
    const callback = jest.fn();

    store.touch('sid-1', sessionData, callback);

    expect(touch).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith(null);
  });

  it('writes again at the interval boundary without extending it on skipped touches', () => {
    store.touch('sid-1', sessionData);
    now.mockReturnValue(299999);
    store.touch('sid-1', sessionData);
    now.mockReturnValue(300000);
    store.touch('sid-1', sessionData);
    now.mockReturnValue(300001);
    store.touch('sid-1', sessionData);

    expect(touch).toHaveBeenCalledTimes(2);
  });

  it('respects a custom touch interval and tracks each session separately', () => {
    store = new ThrottledSessionStore({
      pruneSessionInterval: false,
      touchIntervalMs: 1000,
    });
    store.touch('sid-1', sessionData);
    now.mockReturnValue(999);
    store.touch('sid-1', sessionData);
    store.touch('sid-2', sessionData);
    expect(touch).toHaveBeenCalledTimes(2);

    now.mockReturnValue(1000);
    store.touch('sid-1', sessionData);
    expect(touch).toHaveBeenCalledTimes(3);
  });

  it('forgets destroyed sessions so the next touch writes again', () => {
    store.touch('sid-1', sessionData);
    const callback = jest.fn();

    store.destroy('sid-1', callback);
    store.touch('sid-1', sessionData);

    expect(destroy).toHaveBeenCalledWith('sid-1', callback);
    expect(callback).toHaveBeenCalledWith(null);
    expect(touch).toHaveBeenCalledTimes(2);
  });

  it('evicts the oldest tracked session when maxTrackedSessions is reached', () => {
    store = new ThrottledSessionStore({
      pruneSessionInterval: false,
      maxTrackedSessions: 2,
    });
    store.touch('sid-1', sessionData);
    store.touch('sid-2', sessionData);
    store.touch('sid-3', sessionData);
    touch.mockClear();

    store.touch('sid-2', sessionData);
    store.touch('sid-3', sessionData);
    expect(touch).not.toHaveBeenCalled();

    store.touch('sid-1', sessionData);
    expect(touch).toHaveBeenCalledTimes(1);
    expect(touch).toHaveBeenCalledWith('sid-1', sessionData, undefined);
  });
});
