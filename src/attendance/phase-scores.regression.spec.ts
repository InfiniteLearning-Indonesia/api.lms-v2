import { Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { Between, FindOperator, Repository } from 'typeorm';
import { AttendanceService } from './attendance.service';
import { Attendance, AttendanceStatus } from './entities/attendance.entity';
import { Holiday } from './entities/holiday.entity';
import { PermissionRequest } from './entities/permission-request.entity';
import { Batch } from '../classes/entities/batch.entity';
import { MentorAsyncDay } from '../classes/entities/mentor-async-day.entity';
import { User } from '../users/entities/user.entity';
import { StorageService } from '../storage/storage.service';

type Details = Awaited<
  ReturnType<AttendanceService['calculateStudentPhaseScore']>
>;
type StudentScores = {
  microScore: number;
  massiveScore: number;
  microDetails: Details;
  massiveDetails: Details;
};

// Frozen pre-refactor implementation. Only scoresMap's type is narrowed for lint;
// date iteration, repository queries, phase fallbacks and scoring are unchanged.
class LegacyPhaseScores {
  constructor(
    private batchRepository: Pick<Repository<Batch>, 'findOne'>,
    private attendanceRepository: Pick<Repository<Attendance>, 'find'>,
    private mentorAsyncDayRepository: Pick<Repository<MentorAsyncDay>, 'find'>,
    readonly getHolidays: (year: number) => Promise<Holiday[]>,
  ) {}

  async getActiveDaysForDateRange(
    batchId: string,
    startDate?: Date | null,
    endDate?: Date | null,
  ): Promise<{ total: number; days: Date[]; activeDateStrings: string[] }> {
    const batch = await this.batchRepository.findOne({
      where: { id: batchId },
    });
    const start = startDate
      ? new Date(startDate)
      : batch?.startDate
        ? new Date(batch.startDate)
        : null;
    const end = endDate
      ? new Date(endDate)
      : batch?.endDate
        ? new Date(batch.endDate)
        : null;

    if (!start || !end) {
      return { total: 0, days: [], activeDateStrings: [] };
    }

    const y = start.getFullYear();
    const holidays = await this.getHolidays(y);
    const holidayDates = holidays.map((h) => {
      const d = new Date(h.date);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    });

    const mentorAsyncDays = await this.mentorAsyncDayRepository.find();
    const asyncDateStrings = mentorAsyncDays.map((a) => a.date);

    const activeDays: Date[] = [];
    const activeDateStrings: string[] = [];

    const current = new Date(start);
    current.setHours(12, 0, 0, 0);
    const endLimit = new Date(end);
    endLimit.setHours(23, 59, 59, 999);

    while (current <= endLimit) {
      const dayOfWeek = current.getDay();
      // Skip weekends (0 = Sunday, 6 = Saturday) and Fridays (5 = Asynchronous Off-day)
      if (dayOfWeek !== 0 && dayOfWeek !== 6 && dayOfWeek !== 5) {
        const dateString = `${current.getFullYear()}-${String(current.getMonth() + 1).padStart(2, '0')}-${String(current.getDate()).padStart(2, '0')}`;
        // Skip national holidays & mentor async days
        if (
          !holidayDates.includes(dateString) &&
          !asyncDateStrings.includes(dateString)
        ) {
          activeDays.push(new Date(current));
          activeDateStrings.push(dateString);
        }
      }
      current.setDate(current.getDate() + 1);
    }

    return { total: activeDays.length, days: activeDays, activeDateStrings };
  }

  async calculateStudentPhaseScore(
    batchId: string,
    studentId: string,
    startDate?: Date | null,
    endDate?: Date | null,
  ): Promise<{
    totalSyncDays: number;
    alphaDays: number;
    cleanAttendance: number;
    score: number;
    oncamDays: number;
    oncamScore: number;
  }> {
    const today = new Date();
    const effectiveEndDate =
      endDate && new Date(endDate) < today ? endDate : today;

    const activeInfo = await this.getActiveDaysForDateRange(
      batchId,
      startDate,
      effectiveEndDate,
    );
    const totalSyncDays = activeInfo.total;

    if (totalSyncDays === 0) {
      return {
        totalSyncDays: 0,
        alphaDays: 0,
        cleanAttendance: 0,
        score: 65.0,
        oncamDays: 0,
        oncamScore: 65.0,
      };
    }

    const start = startDate ? new Date(startDate) : new Date(0);
    const end = endDate ? new Date(endDate) : new Date(2099, 11, 31);
    start.setHours(0, 0, 0, 0);
    end.setHours(23, 59, 59, 999);

    const attendances = await this.attendanceRepository.find({
      where: {
        studentId,
        batchId,
        date: Between(start, end),
      },
    });

    const alphaDays = attendances.filter((a) => {
      if (a.status !== AttendanceStatus.ALPHA) return false;
      const d = new Date(a.date);
      const ds = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      return activeInfo.activeDateStrings.includes(ds);
    }).length;

    const oncamDays = attendances.filter((a) => {
      if (a.status !== AttendanceStatus.HADIR_ON_CAM) return false;
      const d = new Date(a.date);
      const ds = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      return activeInfo.activeDateStrings.includes(ds);
    }).length;

    const cleanAttendance = Math.max(0, totalSyncDays - alphaDays);
    const rawScore = (cleanAttendance / totalSyncDays) * 95;
    const score = Math.max(
      65.0,
      Math.min(95.0, Math.round(rawScore * 10) / 10),
    );

    const rawOncamScore = 65.0 + (oncamDays / totalSyncDays) * 30;
    const oncamScore = Math.max(
      65.0,
      Math.min(95.0, Math.round(rawOncamScore * 10) / 10),
    );

    return {
      totalSyncDays,
      alphaDays,
      cleanAttendance,
      score,
      oncamDays,
      oncamScore,
    };
  }

  async getBatchPhaseAttendanceScores(batchId: string) {
    const batch = await this.batchRepository.findOne({
      where: { id: batchId },
    });
    if (!batch) return {};

    const microStart = batch.microStartDate || batch.startDate;
    let microEnd = batch.microEndDate;
    let massiveStart = batch.massiveStartDate;
    const massiveEnd = batch.massiveEndDate || batch.endDate;

    // If microEndDate / massiveStartDate not explicitly set, calculate midpoint
    if (!microEnd && batch.startDate && batch.endDate) {
      const startTime = new Date(batch.startDate).getTime();
      const endTime = new Date(batch.endDate).getTime();
      const midTime = startTime + (endTime - startTime) / 2;
      microEnd = new Date(midTime);
      massiveStart = new Date(midTime + 86400000);
    }

    // Get all attendances in this batch
    const attendances = await this.attendanceRepository.find({
      where: { batchId },
    });
    const studentIds = Array.from(new Set(attendances.map((a) => a.studentId)));

    const scoresMap: Record<string, StudentScores> = {};

    for (const studentId of studentIds) {
      const microRes = await this.calculateStudentPhaseScore(
        batchId,
        studentId,
        microStart,
        microEnd,
      );
      const massiveRes = await this.calculateStudentPhaseScore(
        batchId,
        studentId,
        massiveStart,
        massiveEnd,
      );

      scoresMap[studentId] = {
        microScore: microRes.score,
        massiveScore: massiveRes.score,
        microDetails: microRes,
        massiveDetails: massiveRes,
      };
    }

    return {
      scores: scoresMap,
      phaseDates: {
        microStartDate: microStart,
        microEndDate: microEnd,
        massiveStartDate: massiveStart,
        massiveEndDate: massiveEnd,
      },
    };
  }
}

type Fixture = {
  batch: Batch | null;
  attendances: Attendance[];
  holidays: Holiday[];
  asyncDays: MentorAsyncDay[];
};
type AttendanceQuery = {
  where: {
    batchId: string;
    studentId?: string;
    date?: FindOperator<Date>;
  };
};

const date = (value: string) => new Date(value + 'T12:00:00');
const row = (
  day: string,
  status = AttendanceStatus.HADIR_ON_CAM,
  studentId = 'student-0',
  batchId = 'batch',
): Attendance =>
  Object.assign(new Attendance(), {
    batchId,
    studentId,
    date: date(day),
    status,
  });

function fixture(): Fixture {
  return {
    batch: Object.assign(new Batch(), {
      id: 'batch',
      startDate: date('2026-01-05'),
      endDate: date('2026-01-29'),
      microStartDate: date('2026-01-05'),
      microEndDate: date('2026-01-15'),
      massiveStartDate: date('2026-01-19'),
      massiveEndDate: date('2026-01-29'),
    }),
    attendances: [
      row('2026-01-05'),
      row('2026-01-06', AttendanceStatus.ALPHA),
      row('2026-01-07', AttendanceStatus.HADIR_OFF_CAM),
      row('2026-01-08', AttendanceStatus.IZIN_SAKIT),
      row('2026-01-09', AttendanceStatus.ALPHA), // Friday
      row('2026-01-10', AttendanceStatus.ALPHA), // Saturday
      row('2026-01-11', AttendanceStatus.ALPHA), // Sunday
      row('2026-01-12', AttendanceStatus.ALPHA), // Holiday
      row('2026-01-13', AttendanceStatus.ALPHA), // Mentor async
      row('2026-01-19'),
      row('2026-01-20', AttendanceStatus.ALPHA),
      row('2026-01-21', AttendanceStatus.HADIR_OFF_CAM),
      row('2026-01-22', AttendanceStatus.IZIN_SAKIT),
      row('2026-01-29'),
      row('2026-02-02', AttendanceStatus.ALPHA), // Outside phase
      row('2026-01-05', AttendanceStatus.ALPHA, 'other-student', 'other-batch'),
    ],
    holidays: [
      Object.assign(new Holiday(), { date: date('2026-01-12') }),
      Object.assign(new Holiday(), { date: date('2025-12-31') }),
      Object.assign(new Holiday(), { date: date('2026-01-01') }),
    ],
    asyncDays: [
      Object.assign(new MentorAsyncDay(), {
        mentorId: 'unrelated-mentor',
        date: '2026-01-13',
      }),
    ],
  };
}

// All repository calls are in-memory. In particular count() keeps syncHolidays()
// on its already-populated branch, so the real getHolidays() can be exercised.
function harness(data: Fixture) {
  const batchRepository = {
    findOne: jest.fn(({ where }: { where: { id: string } }) =>
      Promise.resolve(data.batch?.id === where.id ? data.batch : null),
    ),
  };
  const attendanceRepository = {
    find: jest.fn(({ where }: AttendanceQuery) =>
      Promise.resolve(
        data.attendances.filter((attendance) => {
          if (attendance.batchId !== where.batchId) return false;
          if (where.studentId && attendance.studentId !== where.studentId)
            return false;
          if (where.date) {
            const [start, end] = where.date.value as [Date, Date];
            const day = new Date(attendance.date);
            if (day < start || day > end) return false;
          }
          return true;
        }),
      ),
    ),
  };
  const holidayRepository = {
    count: jest.fn(() => Promise.resolve(1)),
    find: jest.fn(({ where }: { where: { date: FindOperator<Date> } }) => {
      const [start, end] = where.date.value as [Date, Date];
      return Promise.resolve(
        data.holidays.filter((holiday) => {
          const day = new Date(holiday.date);
          return day >= start && day <= end;
        }),
      );
    }),
  };
  const mentorAsyncDayRepository = {
    find: jest.fn(() => Promise.resolve(data.asyncDays)),
  };
  const service = new AttendanceService(
    attendanceRepository as unknown as Repository<Attendance>,
    holidayRepository as unknown as Repository<Holiday>,
    {} as Repository<PermissionRequest>,
    batchRepository as unknown as Repository<Batch>,
    {} as Repository<User>,
    mentorAsyncDayRepository as unknown as Repository<MentorAsyncDay>,
    {} as StorageService,
    { get: () => 'test-api-key' } as unknown as ConfigService,
  );
  const legacy = new LegacyPhaseScores(
    batchRepository,
    attendanceRepository,
    mentorAsyncDayRepository,
    (year) => service.getHolidays(year),
  );
  return {
    service,
    legacy,
    batchRepository,
    attendanceRepository,
    holidayRepository,
    mentorAsyncDayRepository,
    reads: () =>
      batchRepository.findOne.mock.calls.length +
      attendanceRepository.find.mock.calls.length +
      holidayRepository.count.mock.calls.length +
      holidayRepository.find.mock.calls.length +
      mentorAsyncDayRepository.find.mock.calls.length,
  };
}

type Scenario = {
  name: string;
  change?: (data: Fixture) => void;
  expectedStudents?: number;
  holidayYears?: number[];
  expectedReads?: number;
};

const scenarios: Scenario[] = [
  {
    name: '110 students with holidays, async days and every attendance status',
    change: (data) => {
      const rows = data.attendances.filter((a) => a.batchId === 'batch');
      data.attendances = Array.from({ length: 110 }, (_, i) =>
        rows.map((attendance) => ({
          ...attendance,
          studentId: 'student-' + i,
        })),
      ).flat();
    },
    expectedStudents: 110,
    holidayYears: [2026],
    expectedReads: 5,
  },
  {
    name: 'midpoint fallback overwrites the existing massive start',
    change: (data) => {
      data.batch!.microEndDate = null;
      data.batch!.massiveStartDate = date('2026-01-27');
    },
  },
  {
    name: 'null phase starts fall back to batch start',
    change: (data) => {
      data.batch!.microStartDate = null;
      data.batch!.massiveStartDate = null;
    },
  },
  {
    name: 'null batch start with an explicit micro start and missing massive start',
    change: (data) => {
      data.batch!.startDate = null;
      data.batch!.massiveStartDate = null;
    },
  },
  {
    name: 'all start dates null',
    change: (data) => {
      data.batch!.startDate = null;
      data.batch!.microStartDate = null;
      data.batch!.massiveStartDate = null;
    },
    holidayYears: [],
  },
  {
    name: 'phases containing only Friday and weekend dates',
    change: (data) => {
      Object.assign(data.batch!, {
        microStartDate: date('2026-01-09'),
        microEndDate: date('2026-01-11'),
        massiveStartDate: date('2026-01-16'),
        massiveEndDate: date('2026-01-18'),
      });
    },
  },
  {
    name: 'future phases',
    change: (data) => {
      Object.assign(data.batch!, {
        microStartDate: date('2026-03-02'),
        microEndDate: date('2026-03-05'),
        massiveStartDate: date('2026-03-09'),
        massiveEndDate: date('2026-03-12'),
      });
    },
  },
  {
    name: 'December 2025 through January 2026 retains start-year holiday lookup',
    change: (data) => {
      Object.assign(data.batch!, {
        startDate: date('2025-12-29'),
        microStartDate: date('2025-12-29'),
        microEndDate: date('2026-01-08'),
        massiveStartDate: date('2026-01-12'),
      });
      data.attendances.push(
        row('2025-12-29'),
        row('2025-12-31', AttendanceStatus.ALPHA),
        row('2026-01-01', AttendanceStatus.ALPHA),
      );
    },
    holidayYears: [2025, 2026],
    expectedReads: 7,
  },
  {
    name: 'duplicate alpha and oncam rows retain counting and score clamps',
    change: (data) => {
      data.attendances.push(
        ...Array.from({ length: 20 }, () =>
          row('2026-01-06', AttendanceStatus.ALPHA),
        ),
        ...Array.from({ length: 20 }, () => row('2026-01-05')),
      );
    },
  },
  {
    name: 'no attendance rows',
    change: (data) => {
      data.attendances = [];
    },
    expectedStudents: 0,
    holidayYears: [],
    expectedReads: 2,
  },
  {
    name: 'batch not found',
    change: (data) => {
      data.batch = null;
    },
    expectedStudents: 0,
    holidayYears: [],
    expectedReads: 1,
  },
  {
    name: 'null phase ends and null batch end use today',
    change: (data) => {
      data.batch!.microEndDate = null;
      data.batch!.massiveEndDate = null;
      data.batch!.endDate = null;
    },
  },
  {
    name: 'ongoing phases exclude attendance after today but include today',
    change: (data) => {
      data.batch!.massiveEndDate = date('2026-02-26');
      data.attendances.push(
        row('2026-02-16'),
        row('2026-02-17', AttendanceStatus.ALPHA),
      );
    },
  },
  {
    name: 'end before start',
    change: (data) => {
      data.batch!.microEndDate = date('2026-01-01');
      data.batch!.massiveEndDate = date('2026-01-01');
    },
  },
  {
    name: 'timestamp attendance at local day boundaries',
    change: (data) => {
      for (const timestamp of [
        '2026-01-05T00:00:00',
        '2026-01-15T23:59:59.999',
        '2026-01-16T00:00:00',
      ]) {
        data.attendances.push({
          ...row('2026-01-05', AttendanceStatus.ALPHA),
          date: new Date(timestamp),
        });
      }
    },
  },
];

describe('getBatchPhaseAttendanceScores regression (no DB)', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-02-16T12:00:00'));
    jest.spyOn(Logger.prototype, 'log').mockImplementation(() => undefined);
    jest
      .spyOn(globalThis, 'fetch')
      .mockRejectedValue(
        new Error('Network access is forbidden in this suite'),
      );
  });

  afterEach(() => {
    expect(globalThis.fetch).not.toHaveBeenCalled();
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it.each(scenarios)('$name', async (scenario) => {
    const data = fixture();
    scenario.change?.(data);
    const old = harness(data);
    const current = harness(data);
    const expected = await old.legacy.getBatchPhaseAttendanceScores('batch');
    const actual = await current.service.getBatchPhaseAttendanceScores('batch');

    expect(actual).toStrictEqual(expected);
    expect(current.reads()).toBeLessThanOrEqual(8);
    if (scenario.expectedReads !== undefined)
      expect(current.reads()).toBe(scenario.expectedReads);
    expect(Object.keys(actual.scores ?? {})).toHaveLength(
      scenario.expectedStudents ?? 1,
    );
    if (scenario.holidayYears) {
      expect(
        current.holidayRepository.find.mock.calls.map(([options]) => {
          const [start] = options.where.date.value as [Date, Date];
          return start.getFullYear();
        }),
      ).toEqual(scenario.holidayYears);
    }
    if (scenario.expectedStudents === 110) {
      expect(current.attendanceRepository.find).toHaveBeenCalledTimes(1);
      expect(current.batchRepository.findOne).toHaveBeenCalledTimes(1);
      expect(current.mentorAsyncDayRepository.find).toHaveBeenCalledTimes(1);
      expect(old.reads()).toBeGreaterThan(880);
    }
  });

  it('preserves getActiveDaysForDateRange output after extracting the helper', async () => {
    const data = fixture();
    const old = harness(data);
    const current = harness(data);
    for (const [start, end] of [
      [undefined, undefined],
      [date('2025-12-29'), date('2026-01-15')],
      [date('2026-01-09'), date('2026-01-11')],
      [date('2026-01-29'), date('2026-01-05')],
    ]) {
      expect(
        await current.service.getActiveDaysForDateRange('batch', start, end),
      ).toStrictEqual(
        await old.legacy.getActiveDaysForDateRange('batch', start, end),
      );
    }
  });

  it('does not reuse the holiday cache across requests', async () => {
    const data = fixture();
    const current = harness(data);
    await current.service.getBatchPhaseAttendanceScores('batch');
    data.holidays.push(
      Object.assign(new Holiday(), { date: date('2026-01-06') }),
    );
    const actual = await current.service.getBatchPhaseAttendanceScores('batch');
    const old = harness(data);
    expect(actual).toStrictEqual(
      await old.legacy.getBatchPhaseAttendanceScores('batch'),
    );
    expect(current.holidayRepository.find).toHaveBeenCalledTimes(2);
  });
});
