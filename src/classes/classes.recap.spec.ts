import { ForbiddenException, NotFoundException } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { DataSource, SelectQueryBuilder } from 'typeorm';
import { ClassesController } from './classes.controller';
import { ClassesService } from './classes.service';
import { computeGrading, GradingAssessment, GradingInput } from './grading';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity';
import { Material } from './entities/material.entity';
import { Assignment } from './entities/assignment.entity';
import { Competency } from './entities/competency.entity';
import { RubrikAssessment } from './entities/rubrik-assessment.entity';
import { RubrikAssessmentScore } from './entities/rubrik-assessment-score.entity';
import { Program } from './entities/program.entity';
import { ProgramHead } from './entities/program-head.entity';
import { Batch, BatchStatus } from './entities/batch.entity';
import { User } from '../users/entities/user.entity';
import { Submission } from './entities/submission.entity';
import { Logbook, LogbookStatus } from './entities/logbook.entity';
import { MentorAsyncDay } from './entities/mentor-async-day.entity';
import { ProgramCompetency } from './entities/program-competency.entity';
import { CompetencyScore } from './entities/competency-score.entity';
import { AiEvaluatorService } from './services/ai-evaluator.service';

function assessment(
  id: string,
  phase = 'Micro',
  competencyId = id,
): GradingAssessment {
  return {
    id,
    name: id,
    phase,
    competencies: [{ competencyId, name: competencyId, weight: 1 }],
  };
}

function grade(overrides: Partial<GradingInput> = {}) {
  return computeGrading({
    rubrikAssessments: [],
    competencyScores: [],
    assignmentScores: new Map(),
    externalScores: new Map(),
    ...overrides,
  });
}

describe('computeGrading', () => {
  it('uses direct scores before assignment scores, and assignments before the minimum', () => {
    const input = {
      rubrikAssessments: [assessment('coding')],
      assignmentScores: new Map([['coding', 90]]),
    };
    expect(grade(input).microItems[0].score).toBe(90);
    expect(
      grade({
        ...input,
        competencyScores: [{ competencyId: 'coding', score: 75 }],
      }).microItems[0].score,
    ).toBe(75);
  });

  it('uses external scores before direct and assignment scores', () => {
    const result = grade({
      rubrikAssessments: [assessment('coding')],
      competencyScores: [{ competencyId: 'coding', score: 75 }],
      assignmentScores: new Map([['coding', 90]]),
      externalScores: new Map([['coding', 98.26]]),
    });
    expect(result.microItems[0].score).toBe(98.3);
  });

  it('combines weighted competencies and recursively weighted subassessments, including child imports', () => {
    const result = grade({
      rubrikAssessments: [
        assessment('leaf'),
        {
          id: 'middle',
          name: 'middle',
          subAssessments: [{ assessmentId: 'leaf', weight: 1 }],
        },
        {
          id: 'root',
          name: 'root',
          phase: 'Massive',
          competencies: [{ competencyId: 'direct', weight: '0.5' }],
          subAssessments: [{ assessmentId: 'middle', weight: 0.5 }],
        },
      ],
      competencyScores: [
        { competencyId: 'leaf', score: 70 },
        { competencyId: 'direct', score: 80 },
      ],
      externalScores: new Map([['leaf', 100]]),
    });
    expect(result.massiveItems[0].score).toBe(90);
    expect(result.missingMassiveCount).toBe(0);
  });

  it('averages displayed phase items, then applies 40/60 weighting and categories', () => {
    const result = grade({
      rubrikAssessments: [
        { ...assessment('communication'), phase: undefined },
        assessment('code'),
        {
          ...assessment('project', 'Massive'),
          programCompetency: { category: 'Soft Skills' },
        },
        assessment('final', 'Massive'),
      ],
      externalScores: new Map([
        ['communication', 70],
        ['code', 90],
        ['project', 80],
        ['final', 100],
      ]),
    });
    expect(result.totalMicroScore).toBe(80);
    expect(result.totalMassiveScore).toBe(90);
    expect(result.finalScore).toBe(86);
    expect(result.predicate).toBe('Very Good');
    expect(result.microItems.map((item) => item.category)).toEqual([
      'Soft Skill',
      'Hard Skill',
    ]);
    expect(result.massiveItems[0].category).toBe('Soft Skill');
  });

  it.each([
    [90, 'With Distinction'],
    [89.9, 'Very Good'],
    [80, 'Very Good'],
    [79.9, 'Good'],
    [70, 'Good'],
    [69.9, 'Satisfactory'],
  ])('maps final score %s to %s', (score, predicate) => {
    const result = grade({
      rubrikAssessments: [
        assessment('micro'),
        assessment('massive', 'Massive'),
      ],
      externalScores: new Map([
        ['micro', score],
        ['massive', score],
      ]),
    });
    expect(result.finalScore).toBe(score);
    expect(result.predicate).toBe(predicate);
  });

  it('clamps and rounds displayed scores, with 65 for empty phases', () => {
    expect(grade()).toMatchObject({
      totalMicroScore: 65,
      totalMassiveScore: 65,
      finalScore: 65,
    });
    const values = [-10, 0, NaN, 101, 81.26];
    const result = grade({
      rubrikAssessments: values.map((_, i) => assessment(String(i))),
      externalScores: new Map(values.map((score, i) => [String(i), score])),
    });
    expect(result.microItems.map((item) => item.score)).toEqual([
      65, 65, 65, 100, 81.3,
    ]);
  });

  it('counts Massive items without any source and treats zero as an entered score', () => {
    const result = grade({
      rubrikAssessments: [
        assessment('missing', 'Massive'),
        assessment('direct', 'Massive'),
        assessment('assignment', 'Massive'),
        assessment('external', 'Massive'),
        {
          id: 'parent',
          name: 'parent',
          phase: 'Massive',
          subAssessments: [{ assessmentId: 'direct', weight: 1 }],
        },
        assessment('micro-missing'),
      ],
      competencyScores: [{ competencyId: 'direct', score: 0 }],
      assignmentScores: new Map([['assignment', 0]]),
      externalScores: new Map([['external', 0]]),
    });
    expect(result.missingMassiveCount).toBe(1);
  });

  it('handles cyclic and dangling subassessment references without recursion errors', () => {
    const result = grade({
      rubrikAssessments: [
        {
          id: 'a',
          name: 'a',
          phase: 'Massive',
          subAssessments: [{ assessmentId: 'b', weight: 1 }],
        },
        {
          id: 'b',
          name: 'b',
          phase: 'Massive',
          subAssessments: [
            { assessmentId: 'a', weight: 1 },
            { assessmentId: 'gone', weight: 1 },
          ],
        },
      ],
    });
    expect(result.missingMassiveCount).toBe(2);
    expect(result.finalScore).toBe(65);
  });
});

// Build TypeORM metadata and SQL locally. No initialize(), connection or DB.
class OfflineDataSource extends DataSource {
  prepare() {
    return this.buildMetadatas();
  }
}

describe('ClassesService.findProgramRecap', () => {
  const entities = [
    Enrollment,
    Class,
    Material,
    Assignment,
    Competency,
    RubrikAssessment,
    RubrikAssessmentScore,
    Program,
    Batch,
    User,
    Submission,
    Logbook,
    MentorAsyncDay,
    ProgramCompetency,
    CompetencyScore,
    ProgramHead,
  ];
  const dataSource = new OfflineDataSource({ type: 'postgres', entities });
  const batch = {
    id: 'batch-1',
    name: 'Batch 1',
    status: BatchStatus.ACTIVE,
    startDate: new Date(2026, 6, 1),
    endDate: new Date(2026, 7, 31),
  };
  const releasedAt = new Date('2026-10-29T00:00:00Z');
  const makeClass = (id = 'class-1') => ({
    id,
    programId: 'program-1',
    batchId: batch.id,
    batch,
    mentor: { id: 'mentor-1', name: 'Mentor' },
    isTranscriptReleased: true,
    transcriptReleasedAt: releasedAt,
    transcriptReleasedBy: 'admin-1',
    isCertificateReleased: false,
  });
  const makeEnrollment = (index = 1) => ({
    studentId: `student-${index}`,
    name: `Student ${index}`,
    roles: ['student'],
    classId: 'class-1',
    competencyScores: [{ competencyId: 'coding', score: 80 }],
    externalScores: [] as { rubrikAssessmentId: string; score: number }[],
    logbooks: [{ monthIndex: 1, status: LogbookStatus.ACCEPTED }],
  });
  let context: {
    id: string;
    name: string;
    recapHeads: { userId: string }[];
    recapClasses: ReturnType<typeof makeClass>[];
  } | null;
  let enrollments: ReturnType<typeof makeEnrollment>[];
  let submissions: unknown[];
  let service: ClassesService;
  let queries: string[];
  let enrollmentQuery: SelectQueryBuilder<Enrollment>;

  beforeAll(() => dataSource.prepare());

  beforeEach(async () => {
    queries = [];
    context = {
      id: 'program-1',
      name: 'Web',
      recapHeads: [],
      recapClasses: [makeClass()],
    };
    enrollments = [makeEnrollment()];
    submissions = [];
    const programRepository = dataSource.getRepository(Program);
    jest
      .spyOn(programRepository, 'createQueryBuilder')
      .mockImplementation((alias) => {
        const qb = dataSource.createQueryBuilder(Program, alias!);
        jest.spyOn(qb, 'getOne').mockImplementation(() => {
          queries.push(qb.getQueryAndParameters()[0]);
          return Promise.resolve(context as unknown as Program | null);
        });
        return qb;
      });
    const enrollmentRepository = dataSource.getRepository(Enrollment);
    jest
      .spyOn(enrollmentRepository, 'createQueryBuilder')
      .mockImplementation((alias) => {
        const qb = dataSource.createQueryBuilder(Enrollment, alias!);
        enrollmentQuery = qb;
        jest.spyOn(qb, 'getRawMany').mockImplementation(() => {
          queries.push(qb.getQueryAndParameters()[0]);
          return Promise.resolve(enrollments);
        });
        return qb;
      });
    const submissionRepository = dataSource.getRepository(Submission);
    jest
      .spyOn(submissionRepository, 'createQueryBuilder')
      .mockImplementation((alias) => {
        const qb = dataSource.createQueryBuilder(Submission, alias!);
        jest.spyOn(qb, 'getMany').mockImplementation(() => {
          queries.push(qb.getQueryAndParameters()[0]);
          return Promise.resolve(submissions as Submission[]);
        });
        return qb;
      });
    jest
      .spyOn(dataSource.getRepository(Competency), 'find')
      .mockImplementation(() => {
        queries.push('competencies.find');
        return Promise.resolve([
          { id: 'coding', name: 'Coding' },
        ] as Competency[]);
      });
    jest
      .spyOn(dataSource.getRepository(RubrikAssessment), 'find')
      .mockImplementation(() => {
        queries.push('rubrikAssessments.find');
        return Promise.resolve([
          assessment('micro', 'Micro', 'coding'),
          assessment('massive', 'Massive', 'coding'),
        ] as RubrikAssessment[]);
      });
    // Unexpected repository SQL fails immediately; only instrumented calls above
    // can run. This also catches writes or hidden per-student repository lookups.
    jest.spyOn(dataSource, 'createQueryRunner').mockImplementation(() => {
      throw new Error('Unexpected database access in recap');
    });
    const module = await Test.createTestingModule({
      providers: [
        ClassesService,
        ...entities.map((entity) => ({
          provide: getRepositoryToken(entity),
          useValue: dataSource.getRepository(entity),
        })),
        { provide: AiEvaluatorService, useValue: {} },
      ],
    }).compile();
    service = module.get(ClassesService);
  });

  afterEach(() => {
    expect(queries.length).toBeLessThanOrEqual(5);
    jest.restoreAllMocks();
  });

  it('registers the recap controller route with admin, facilitator and mentor roles', () => {
    const handler = Reflect.get(
      ClassesController.prototype,
      'getProgramRecap',
    ) as object;
    expect(Reflect.getMetadata('path', handler)).toBe(
      'programs/:programId/recap',
    );
    expect(Reflect.getMetadata('roles', handler)).toEqual([
      'admin',
      'facilitator',
      'mentor',
    ]);
  });

  it('allows admin to release and returns the recap, release metadata and batch logbooks', async () => {
    const result = await service.findProgramRecap('program-1', {
      id: 'admin-1',
      roles: ['admin'],
    });
    expect(result).toMatchObject({
      program: { id: 'program-1', name: 'Web' },
      batch: { id: batch.id, name: batch.name },
      canRelease: true,
      transcriptReleased: { value: true, at: releasedAt, by: 'admin-1' },
      certificateReleased: { value: false, at: null, by: null },
      completeness: {
        totalStudents: 1,
        studentsWithMassiveComplete: 1,
        studentsWithMassiveIncomplete: 0,
      },
      students: [
        {
          id: 'student-1',
          name: 'Student 1',
          classId: 'class-1',
          className: 'Web - Batch 1',
          mentorName: 'Mentor',
          totalMicroScore: 80,
          totalMassiveScore: 80,
          finalScore: 80,
          predicate: 'Very Good',
          missingMassiveCount: 0,
          logbookCompletionStatus: {
            allAccepted: false,
            month1Accepted: true,
            totalRequired: 2,
            totalAccepted: 1,
            firstIncompleteMonth: 2,
          },
        },
      ],
    });
    expect(queries).toHaveLength(5);
    expect(queries[3]).toContain('CAST("student"."roles" AS jsonb)');
    expect(queries[3]).toContain('"lb"."batchId" = "recapClass"."batchId"');
    expect(queries[0]).toContain('"head"."userId" = $1');
  });

  it('allows mentor head to release', async () => {
    context!.recapHeads = [{ userId: 'mentor-1' }];
    const result = await service.findProgramRecap('program-1', {
      id: 'mentor-1',
      roles: ['mentor'],
    });
    expect(result.canRelease).toBe(true);
  });

  it('rejects mentor non-head before loading student data', async () => {
    await expect(
      service.findProgramRecap('program-1', { id: 'other', roles: ['mentor'] }),
    ).rejects.toEqual(
      new ForbiddenException(
        'Hanya Head Program, admin, atau facilitator yang dapat melihat rekap program ini.',
      ),
    );
    expect(queries).toHaveLength(1);
  });

  it.each([['facilitator'], ['mentor', 'facilitator']])(
    'allows %j to view without release rights',
    async (...roles) => {
      const result = await service.findProgramRecap('program-1', {
        id: 'staff',
        roles,
      });
      expect(result.canRelease).toBe(false);
    },
  );

  it('keeps the query count at five for 64 students and calculates completeness', async () => {
    enrollments = Array.from({ length: 64 }, (_, i) => makeEnrollment(i + 1));
    enrollments[0].competencyScores = [];
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(queries).toHaveLength(5);
    expect(result.completeness).toEqual({
      totalStudents: 64,
      studentsWithMassiveComplete: 63,
      studentsWithMassiveIncomplete: 1,
    });
    expect(result.students[0].missingMassiveCount).toBe(1);
  });

  it('derives weighted assignment fallback using competency names, including shared batch classes', async () => {
    enrollments[0].competencyScores = [];
    submissions = [
      {
        studentId: 'student-1',
        score: 70,
        assignment: {
          competency: 'CODING',
          weight: 0.25,
          class: { batchId: batch.id },
        },
      },
      {
        studentId: 'student-1',
        score: 90,
        assignment: {
          competency: 'Coding',
          weight: 0.75,
          class: { batchId: batch.id },
        },
      },
    ];
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.students[0].finalScore).toBe(85);
    expect(result.students[0].missingMassiveCount).toBe(0);
    expect(queries[4]).toContain('"submission"."status" =');
    expect(queries[4]).toContain('"assignmentClass"."batchId" IN');
  });

  it('uses external scores over student direct scores', async () => {
    enrollments[0].externalScores = [
      { rubrikAssessmentId: 'massive', score: 100 },
    ];
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.students[0].finalScore).toBe(92);
  });

  it('selects active classes and excludes historical classes', async () => {
    context!.recapClasses.push({
      ...makeClass('old-class'),
      batchId: 'old-batch',
      batch: { ...batch, id: 'old-batch', status: BatchStatus.COMPLETED },
    });
    await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    // Active selection occurs before enrollment/submission queries are built.
    expect(enrollmentQuery.getParameters().classIds).toEqual(['class-1']);
  });

  it('falls back to all batches and keeps assignment/logbook data separate by batch', async () => {
    context!.recapClasses = [
      { ...makeClass(), batch: { ...batch, status: BatchStatus.COMPLETED } },
      {
        ...makeClass('class-2'),
        batchId: 'batch-2',
        batch: { ...batch, id: 'batch-2', status: BatchStatus.COMPLETED },
      },
    ];
    enrollments = [
      makeEnrollment(),
      {
        ...makeEnrollment(),
        classId: 'class-2',
        competencyScores: [],
        logbooks: [],
      },
    ];
    submissions = [
      {
        studentId: 'student-1',
        score: 100,
        assignment: {
          competency: 'Coding',
          weight: 1,
          class: { batchId: batch.id },
        },
      },
    ];
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.batch).toBeNull();
    expect(result.students.map((s) => s.finalScore)).toEqual([80, 65]);
    expect(
      result.students.map((s) => s.logbookCompletionStatus.month1Accepted),
    ).toEqual([true, false]);
    expect(queries).toHaveLength(5);
  });

  it('does not report a program released if only some classes are released', async () => {
    context!.recapClasses.push({
      ...makeClass('class-2'),
      isTranscriptReleased: false,
    });
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.transcriptReleased).toEqual({
      value: false,
      at: null,
      by: null,
    });
  });

  it('deduplicates accepted logbook months and ignores months outside the required range', async () => {
    enrollments[0].logbooks = [1, 1, 2, 99].map((monthIndex) => ({
      monthIndex,
      status: LogbookStatus.ACCEPTED,
    }));
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.students[0].logbookCompletionStatus).toEqual({
      allAccepted: true,
      month1Accepted: true,
      totalRequired: 2,
      totalAccepted: 2,
      firstIncompleteMonth: null,
    });
  });

  it('returns an empty recap without querying submissions if there are no students', async () => {
    enrollments = [{ ...makeEnrollment(), roles: ['mentor'] }];
    const result = await service.findProgramRecap('program-1', {
      id: 'admin',
      roles: ['admin'],
    });
    expect(result.students).toEqual([]);
    expect(queries).toHaveLength(4);
  });

  it('rejects unknown programs and programs without classes', async () => {
    context = null;
    await expect(
      service.findProgramRecap('missing', { id: 'admin', roles: ['admin'] }),
    ).rejects.toEqual(new NotFoundException('Program tidak ditemukan.'));
    context = { id: 'empty', name: 'Empty', recapHeads: [], recapClasses: [] };
    await expect(
      service.findProgramRecap('empty', { id: 'admin', roles: ['admin'] }),
    ).rejects.toEqual(new NotFoundException('Kelas program tidak ditemukan.'));
    expect(queries).toHaveLength(2);
  });
});
