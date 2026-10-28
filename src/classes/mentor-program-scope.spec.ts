import { ForbiddenException } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { In } from 'typeorm';
import { AttendanceService } from '../attendance/attendance.service';
import { Attendance } from '../attendance/entities/attendance.entity';
import { User, UserRole } from '../users/entities/user.entity';
import { ClassesService } from './classes.service';
import { Batch } from './entities/batch.entity';
import { Class } from './entities/class.entity';
import { Enrollment } from './entities/enrollment.entity';
import { Program } from './entities/program.entity';
import { resolveMentorProgramScope } from './mentor-program-scope';

const batchId = 'batch-1';
const mentorId = 'mentor-1';
const batch = Object.assign(new Batch(), { id: batchId });
const classes = [
  ['web-own', 'Web Development and UI/UX Design', mentorId],
  ['web-other', 'Advanced WEB DEVELOPMENT and UI/UX Design', 'mentor-2'],
  ['mobile', 'Mobile Development and UI/UX Design', 'mentor-3'],
  ['game', 'Game Development', 'mentor-4'],
  ['ai', 'AI Development', 'mentor-5'],
  ['other', 'Web Design', 'mentor-6'],
].map(([id, name, owner]) =>
  Object.assign(new Class(), {
    id,
    batchId,
    mentorId: owner,
    programId: `program-${id}`,
    program: Object.assign(new Program(), { id: `program-${id}`, name }),
    batch,
  }),
);
const enrollments = classes.map((cls) =>
  Object.assign(new Enrollment(), {
    classId: cls.id,
    studentId: `student-${cls.id}`,
    student: Object.assign(new User(), {
      id: `student-${cls.id}`,
      name: cls.id,
      roles: [UserRole.STUDENT],
    }),
  }),
);

describe('resolveMentorProgramScope', () => {
  it.each([
    ['Professional', 'all', []],
    ['UI', 'programs', ['web development', 'mobile development']],
    ['UX', 'programs', ['web development', 'mobile development']],
    ['UI/UX Web', 'programs', ['web development', 'mobile development']],
    ['Senior WEB mentor', 'programs', ['web development']],
    ['Game', 'programs', ['game development']],
    ['AI', 'programs', ['ai development']],
    ['Artificial intelligence', 'programs', ['ai development']],
    ['', 'own', []],
    [null, 'own', []],
    [undefined, 'own', []],
    ['Unknown', 'own', []],
  ])('resolves %s to %s', (specialization, mode, patterns) => {
    expect(resolveMentorProgramScope(specialization as string | null)).toEqual({
      mode,
      patterns,
    });
  });
});

describe('mentor program scope in batch recap', () => {
  const batchRepository = { findOne: jest.fn() };
  const classRepository = { find: jest.fn() };
  const enrollmentRepository = { find: jest.fn() };
  let service: ClassesService;

  beforeEach(async () => {
    jest.clearAllMocks();
    batchRepository.findOne.mockResolvedValue(batch);
    classRepository.find.mockResolvedValue(classes);
    enrollmentRepository.find.mockResolvedValue(enrollments);
    const module = await Test.createTestingModule({
      providers: [
        ClassesService,
        { provide: getRepositoryToken(Batch), useValue: batchRepository },
        { provide: getRepositoryToken(Class), useValue: classRepository },
        {
          provide: getRepositoryToken(Enrollment),
          useValue: enrollmentRepository,
        },
      ],
    })
      .useMocker(() => ({}))
      .compile();
    service = module.get(ClassesService);
  });

  it.each([
    ['Web', ['web-own', 'web-other']],
    ['UI', ['web-own', 'web-other', 'mobile']],
    ['UX', ['web-own', 'web-other', 'mobile']],
    ['UI/UX', ['web-own', 'web-other', 'mobile']],
    ['Game', ['game']],
    ['AI', ['ai']],
    ['Professional', classes.map((cls) => cls.id)],
    ['', ['web-own']],
    ['Unknown', ['web-own']],
  ])(
    'limits %s mentors to the expected classes and mentees',
    async (spec, ids) => {
      const result = await service.findBatchRecapClasses(batchId, {
        id: mentorId,
        roles: ['mentor'],
        specialization: spec,
      });

      expect(result.map((cls) => cls.id)).toEqual(ids);
      expect(
        result.flatMap((cls) => cls.enrolledStudents.map((s) => s.id)),
      ).toEqual(ids.map((id) => `student-${id}`));
      expect(enrollmentRepository.find).toHaveBeenCalledWith({
        where: { classId: In(ids) },
        relations: { student: true },
      });
      expect(batchRepository.findOne).toHaveBeenCalledTimes(1);
      expect(classRepository.find).toHaveBeenCalledTimes(1);
      expect(enrollmentRepository.find).toHaveBeenCalledTimes(1);
    },
  );

  it.each(['admin', 'facilitator'])(
    'keeps all programs for %s, including callers with a mentor role',
    async (role) => {
      const result = await service.findBatchRecapClasses(batchId, {
        id: 'staff-1',
        roles: [role, 'mentor'],
        specialization: 'Game',
      });
      expect(result.map((cls) => cls.id)).toEqual(classes.map((cls) => cls.id));
    },
  );

  it.each(['Professional', 'Web', ''])(
    'rejects %s mentors outside the batch',
    async (spec) => {
      await expect(
        service.findBatchRecapClasses(batchId, {
          id: 'outside-mentor',
          roles: ['mentor'],
          specialization: spec,
        }),
      ).rejects.toThrow(ForbiddenException);
      expect(enrollmentRepository.find).not.toHaveBeenCalled();
    },
  );

  it('returns no classes when the authorized mentor has no matching program', async () => {
    classRepository.find.mockResolvedValue([classes[0]]);
    await expect(
      service.findBatchRecapClasses(batchId, {
        id: mentorId,
        roles: ['mentor'],
        specialization: 'Game',
      }),
    ).resolves.toEqual([]);
    expect(enrollmentRepository.find).not.toHaveBeenCalled();
  });
});

// A small in-memory query double checks the emitted SQL and applies its predicates
// to fixtures. No connection, database, or external service is created.
interface AttendanceQueryMock {
  leftJoinAndSelect: jest.Mock;
  leftJoin: jest.Mock;
  orderBy: jest.Mock;
  andWhere: jest.Mock<unknown, [string, Record<string, string>?]>;
  getMany: jest.Mock<Promise<Attendance[]>, []>;
}

function attendanceQuery() {
  type Parameters = Record<string, string>;
  const predicates: ((row: Attendance) => boolean)[] = [];
  const rows = [
    ...enrollments.map((en) =>
      Object.assign(new Attendance(), { studentId: en.studentId, batchId }),
    ),
    // This student's Web enrollment belongs to another batch only.
    Object.assign(new Attendance(), { studentId: 'outside-student', batchId }),
    Object.assign(new Attendance(), {
      studentId: enrollments[0].studentId,
      batchId: 'batch-2',
    }),
  ];
  const membership = [
    ...enrollments.map((en, index) => ({ ...en, class: classes[index] })),
    // Multiple enrollments must not duplicate attendance rows.
    { ...enrollments[0], class: classes[1] },
    {
      studentId: 'outside-student',
      class: { ...classes[0], batchId: 'batch-2' },
    },
  ];
  const isMember = (row: Attendance, matches: (cls: Class) => boolean) =>
    membership.some(
      (en) =>
        en.studentId === row.studentId &&
        en.class.batchId === row.batchId &&
        matches(en.class),
    );
  const query: AttendanceQueryMock = {
    leftJoinAndSelect: jest.fn().mockReturnThis(),
    leftJoin: jest.fn().mockReturnThis(),
    orderBy: jest.fn().mockReturnThis(),
    andWhere: jest.fn<unknown, [string, Parameters?]>(
      (sql, parameters = {}) => {
        switch (sql) {
          case 'attendance.studentId = :callerId':
            predicates.push((row) => row.studentId === parameters.callerId);
            break;
          case 'attendance.studentId = :studentId':
            predicates.push((row) => row.studentId === parameters.studentId);
            break;
          case 'attendance.batchId = :batchId':
            predicates.push((row) => row.batchId === parameters.batchId);
            break;
          case 'cls."batchId" = attendance."batchId"':
            break; // Batch correlation is checked by isMember below.
          case 'cls."mentorId" = :mentorId':
            predicates.push((row) =>
              isMember(row, (cls) => cls.mentorId === parameters.mentorId),
            );
            break;
          default: {
            expect(sql).toContain('EXISTS (');
            expect(sql).toContain('FROM enrollments scope_en');
            expect(sql).toContain('scope_cls.id = scope_en."classId"');
            expect(sql).toContain(
              'scope_en."studentId" = attendance."studentId"',
            );
            expect(sql).toContain('scope_cls."batchId" = attendance."batchId"');
            if ('scopeMentorId' in parameters) {
              expect(sql).toContain('scope_cls."mentorId" = :scopeMentorId');
              predicates.push((row) =>
                isMember(
                  row,
                  (cls) => cls.mentorId === parameters.scopeMentorId,
                ),
              );
            } else {
              expect(sql).toContain('scope_program.id = scope_cls."programId"');
              const keys = Object.keys(parameters);
              expect(keys.length).toBeGreaterThan(0);
              expect(sql).toContain(
                `(${keys.map((key) => `LOWER(scope_program.name) LIKE :${key}`).join(' OR ')})`,
              );
              predicates.push((row) =>
                isMember(row, (cls) =>
                  keys.some((key) =>
                    cls.program.name
                      .toLowerCase()
                      .includes(parameters[key].slice(1, -1)),
                  ),
                ),
              );
            }
          }
        }
        return query;
      },
    ),
    getMany: jest.fn(() =>
      Promise.resolve(
        rows.filter((row) => predicates.every((test) => test(row))),
      ),
    ),
  };
  return query;
}

describe('mentor program scope in attendance', () => {
  let service: AttendanceService;
  let query: ReturnType<typeof attendanceQuery>;
  const mentorLookup = jest.fn();
  const mappingQuery = jest.fn().mockResolvedValue([]);

  beforeEach(async () => {
    jest.clearAllMocks();
    query = attendanceQuery();
    mentorLookup.mockResolvedValue({
      id: 'mentor-2',
      specialization: 'Web',
      roles: [UserRole.MENTOR],
      role: UserRole.MENTOR,
    });
    const module = await Test.createTestingModule({
      providers: [
        AttendanceService,
        {
          provide: getRepositoryToken(Attendance),
          useValue: {
            createQueryBuilder: () => query,
            manager: { query: mappingQuery },
          },
        },
        {
          provide: getRepositoryToken(User),
          useValue: { findOne: mentorLookup },
        },
      ],
    })
      .useMocker(() => ({}))
      .compile();
    service = module.get(AttendanceService);
  });

  it.each([
    ['Web', ['web-own', 'web-other']],
    ['UI', ['web-own', 'web-other', 'mobile']],
    ['UX', ['web-own', 'web-other', 'mobile']],
    ['UI/UX', ['web-own', 'web-other', 'mobile']],
    ['Game', ['game']],
    ['AI', ['ai']],
    ['', ['web-own']],
    ['Unknown', ['web-own']],
    ['Professional', [...classes.map((cls) => cls.id), 'outside']],
  ])(
    'limits %s mentors to the expected students and ignores mentorId',
    async (spec, ids) => {
      const result = await service.getAttendances(
        { id: mentorId, roles: ['mentor'], specialization: spec },
        batchId,
        undefined,
        'mentor-4',
      );

      expect(result.map((row) => row.studentId)).toEqual(
        ids.map((id) =>
          id === 'outside' ? 'outside-student' : `student-${id}`,
        ),
      );
      expect(mentorLookup).not.toHaveBeenCalled();
      expect(query.getMany).toHaveBeenCalledTimes(1);
      expect(mappingQuery).toHaveBeenCalledTimes(1);
      expect(query.leftJoin).not.toHaveBeenCalled();
    },
  );

  it.each(['admin', 'facilitator'])(
    'preserves mentorId filtering for %s with a mentor role',
    async (role) => {
      const result = await service.getAttendances(
        { id: 'staff-1', roles: [role, 'mentor'], specialization: 'Game' },
        batchId,
        undefined,
        'mentor-2',
      );
      expect(result.map((row) => row.studentId)).toEqual([
        'student-web-own',
        'student-web-other',
      ]);
      expect(mentorLookup).toHaveBeenCalledWith({ where: { id: 'mentor-2' } });
      expect(mentorLookup).toHaveBeenCalledTimes(1);
      expect(query.getMany).toHaveBeenCalledTimes(1);
      expect(mappingQuery).toHaveBeenCalledTimes(1);
    },
  );

  it.each(['admin', 'facilitator'])(
    'keeps all batch attendance for %s without mentorId',
    async (role) => {
      const result = await service.getAttendances(
        { id: 'staff-1', roles: [role] },
        batchId,
      );
      expect(result).toHaveLength(classes.length + 1);
      expect(mentorLookup).not.toHaveBeenCalled();
    },
  );

  it('always gives students only their own records, including mixed roles and conflicting parameters', async () => {
    const result = await service.getAttendances(
      { id: 'student-web-own', roles: ['student', 'mentor', 'admin'] },
      batchId,
      'student-mobile',
      'mentor-4',
    );
    expect(result.map((row) => row.studentId)).toEqual(['student-web-own']);
    expect(mentorLookup).not.toHaveBeenCalled();
    expect(query.leftJoin).not.toHaveBeenCalled();
  });

  it('intersects studentId with the mentor program scope', async () => {
    const result = await service.getAttendances(
      { id: mentorId, roles: ['mentor'], specialization: 'Web' },
      batchId,
      'student-game',
    );
    expect(result).toEqual([]);
    expect(mappingQuery).not.toHaveBeenCalled();
  });

  it('correlates the scope with each attendance batch when batchId is omitted', async () => {
    const result = await service.getAttendances({
      id: mentorId,
      roles: ['mentor'],
      specialization: 'Web',
    });
    expect(result.map((row) => [row.studentId, row.batchId])).toEqual([
      ['student-web-own', batchId],
      ['student-web-other', batchId],
    ]);
    expect(mappingQuery).not.toHaveBeenCalled();
  });
});
