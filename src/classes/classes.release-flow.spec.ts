import {
  BadRequestException,
  ForbiddenException,
  NotFoundException,
} from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { EntityManager, In } from 'typeorm';
import { ClassesService } from './classes.service';
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
import { User, UserRole } from '../users/entities/user.entity';
import { Submission } from './entities/submission.entity';
import { Logbook, LogbookStatus } from './entities/logbook.entity';
import { MentorAsyncDay } from './entities/mentor-async-day.entity';
import { ProgramCompetency } from './entities/program-competency.entity';
import { CompetencyScore } from './entities/competency-score.entity';
import { AiEvaluatorService } from './services/ai-evaluator.service';
import { AuditLog } from '../audit/entities/audit-log.entity';

function mockRepository() {
  return {
    find: jest.fn<Promise<unknown[]>, [unknown?]>().mockResolvedValue([]),
    findOne: jest.fn<Promise<unknown>, [unknown?]>().mockResolvedValue(null),
    exists: jest.fn<Promise<boolean>, [unknown?]>().mockResolvedValue(false),
    save: jest.fn<Promise<unknown>, [unknown]>().mockResolvedValue({}),
    insert: jest.fn<Promise<unknown>, [unknown]>().mockResolvedValue({}),
    createQueryBuilder: jest.fn(),
  };
}

const program = { id: 'program-1', name: 'Web' };
const batch = {
  id: 'batch-1',
  name: 'Batch 1',
  status: BatchStatus.ACTIVE,
  startDate: new Date(2026, 6, 1),
  endDate: new Date(2026, 7, 31),
};
const previousRelease = new Date('2026-10-29T00:00:00Z');
const makeClass = (id = 'class-1') => ({
  id,
  programId: program.id,
  batchId: batch.id,
  program,
  batch,
  mentor: { name: 'Mentor', email: 'mentor@example.test' },
  isTranscriptReleased: false,
  transcriptReleasedAt: null as Date | null,
  transcriptReleasedBy: null as string | null,
  isCertificateReleased: false,
  certificateReleasedAt: null as Date | null,
  certificateReleasedBy: null as string | null,
});
const makeEnrollment = (index = 1) => ({
  studentId: `student-${index}`,
  name: `Student ${index}`,
  roles: ['student'],
  classId: 'class-1',
  competencyScores: [
    { competencyId: 'coding', score: 80 },
    { competencyId: 'project', score: 90 },
  ],
  externalScores: [] as { rubrikAssessmentId: string; score: number }[],
  logbooks: [1, 2].map((monthIndex) => ({
    monthIndex,
    status: LogbookStatus.ACCEPTED,
  })),
});

// Query-builder stubs return fixtures only: no DataSource initialization or DB.
function queryBuilder() {
  return {
    leftJoinAndMapMany: jest.fn().mockReturnThis(),
    leftJoinAndSelect: jest.fn().mockReturnThis(),
    leftJoin: jest.fn().mockReturnThis(),
    innerJoin: jest.fn().mockReturnThis(),
    innerJoinAndSelect: jest.fn().mockReturnThis(),
    select: jest.fn().mockReturnThis(),
    addSelect: jest.fn().mockReturnThis(),
    where: jest.fn().mockReturnThis(),
    andWhere: jest.fn().mockReturnThis(),
    setParameters: jest.fn().mockReturnThis(),
    orderBy: jest.fn().mockReturnThis(),
    getOne: jest.fn<Promise<unknown>, []>(),
    getRawMany: jest.fn<Promise<unknown[]>, []>(),
    getMany: jest.fn<Promise<unknown[]>, []>(),
  };
}

describe('ClassesService release flow and grade locks', () => {
  let service: ClassesService;
  let classes: ReturnType<typeof makeClass>[];
  let enrollments: ReturnType<typeof makeEnrollment>[];
  let repositories: Map<unknown, ReturnType<typeof mockRepository>>;
  let transaction: jest.Mock;
  let enrollmentQuery: ReturnType<typeof queryBuilder>;
  let submissionQuery: ReturnType<typeof queryBuilder>;
  let caller: { id: string; email: string; roles: UserRole[] };
  const repo = (entity: unknown) => repositories.get(entity)!;
  const markTranscriptReleased = () => {
    for (const cls of classes) {
      cls.isTranscriptReleased = true;
      cls.transcriptReleasedAt = previousRelease;
      cls.transcriptReleasedBy = 'previous-head';
    }
  };
  const markCertificateReleased = () => {
    markTranscriptReleased();
    for (const cls of classes) {
      cls.isCertificateReleased = true;
      cls.certificateReleasedAt = previousRelease;
      cls.certificateReleasedBy = 'previous-head';
    }
  };

  beforeEach(async () => {
    classes = [makeClass(), makeClass('class-2')];
    enrollments = [makeEnrollment()];
    caller = {
      id: 'head-1',
      email: 'head@example.test',
      roles: [UserRole.MENTOR],
    };
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
      AuditLog,
    ];
    repositories = new Map(
      entities.map((entity) => [entity, mockRepository()]),
    );
    const manager = {
      getRepository: (entity: unknown) => repo(entity),
    } as unknown as EntityManager;
    transaction = jest
      .fn()
      .mockImplementation(
        (work: (manager: EntityManager) => Promise<unknown>) => work(manager),
      );
    const module = await Test.createTestingModule({
      providers: [
        ClassesService,
        ...entities.map((entity) => ({
          provide: getRepositoryToken(entity),
          useValue:
            entity === Class
              ? { ...repo(entity), manager: { dataSource: { transaction } } }
              : repo(entity),
        })),
        { provide: AiEvaluatorService, useValue: {} },
      ],
    }).compile();
    service = module.get(ClassesService);
    repo(User).findOne.mockImplementation((options) => {
      const {
        where: { id },
      } = options as { where: { id: string } };
      return Promise.resolve(
        id.startsWith('student-')
          ? { id, name: id, email: `${id}@example.test` }
          : caller,
      );
    });
    repo(ProgramHead).exists.mockResolvedValue(true);
    repo(Batch).find.mockResolvedValue([batch]);
    repo(Class).find.mockImplementation(() => Promise.resolve(classes));
    repo(Program).findOne.mockResolvedValue(program);
    repo(Competency).find.mockResolvedValue([
      { id: 'coding', name: 'Coding' },
      { id: 'project', name: 'Project' },
    ]);
    repo(RubrikAssessment).find.mockResolvedValue([
      {
        id: 'micro',
        name: 'Coding',
        phase: 'Micro',
        competencies: [{ competencyId: 'coding', weight: 1 }],
      },
      {
        id: 'massive',
        name: 'Project',
        phase: 'Massive',
        competencies: [{ competencyId: 'project', weight: 1 }],
      },
    ]);
    enrollmentQuery = queryBuilder();
    let queriedStudentId: string | undefined;
    enrollmentQuery.andWhere.mockImplementation(
      (_sql: string, params: { studentId: string }) => {
        queriedStudentId = params.studentId;
        return enrollmentQuery;
      },
    );
    enrollmentQuery.getRawMany.mockImplementation(() =>
      Promise.resolve(
        enrollments.filter(
          (e) => !queriedStudentId || e.studentId === queriedStudentId,
        ),
      ),
    );
    repo(Enrollment).createQueryBuilder.mockReturnValue(enrollmentQuery);
    repo(Enrollment).find.mockImplementation(() =>
      Promise.resolve([{ class: classes[0], studentId: 'student-1' }]),
    );
    submissionQuery = queryBuilder();
    submissionQuery.getMany.mockResolvedValue([]);
    repo(Submission).createQueryBuilder.mockReturnValue(submissionQuery);
    const programQuery = queryBuilder();
    programQuery.getOne.mockImplementation(() =>
      Promise.resolve({
        ...program,
        recapHeads: [],
        recapClasses: classes,
      }),
    );
    repo(Program).createQueryBuilder.mockReturnValue(programQuery);
  });

  it.each(['transcript', 'certificate'] as const)(
    'rejects non-head mentors for %s with 403',
    async (kind) => {
      repo(ProgramHead).exists.mockResolvedValue(false);
      await expect(
        kind === 'transcript'
          ? service.releaseTranscript(caller.id, program.id)
          : service.releaseCertificate(caller.id, program.id),
      ).rejects.toEqual(
        new ForbiddenException(
          'Hanya Head Program untuk program ini atau admin yang dapat merilis atau menarik rilis dokumen.',
        ),
      );
      expect(repo(ProgramHead).exists).toHaveBeenCalledWith({
        where: { programId: program.id, userId: caller.id },
      });
      expect(repo(Class).find).not.toHaveBeenCalled();
      expect(repo(Class).save).not.toHaveBeenCalled();
      expect(repo(AuditLog).insert).not.toHaveBeenCalled();
    },
  );

  it.each(['head', 'admin'])(
    'allows %s to release both documents with metadata and an audit in one transaction each',
    async (role) => {
      if (role === 'admin') {
        caller.roles = [UserRole.ADMIN];
        repo(ProgramHead).exists.mockResolvedValue(false);
      }
      const transcript = await service.releaseTranscript(caller.id, program.id);
      expect(transcript).toMatchObject({
        success: true,
        isTranscriptReleased: true,
        isCertificateReleased: false,
        releasedAt: expect.any(Date),
        releasedBy: caller.id,
      });
      for (const cls of classes) {
        expect(cls).toMatchObject({
          transcriptReleasedAt: transcript.releasedAt,
          transcriptReleasedBy: caller.id,
        });
      }
      const certificate = await service.releaseCertificate(
        caller.id,
        program.id,
      );
      expect(certificate).toMatchObject({
        isTranscriptReleased: true,
        isCertificateReleased: true,
        releasedAt: expect.any(Date),
        releasedBy: caller.id,
        incompleteStudents: [],
      });
      for (const cls of classes) {
        expect(cls).toMatchObject({
          certificateReleasedAt: certificate.releasedAt,
          certificateReleasedBy: caller.id,
        });
      }
      expect(transaction).toHaveBeenCalledTimes(2);
      expect(repo(Class).save).toHaveBeenCalledTimes(2);
      for (const action of ['RELEASE_TRANSCRIPT', 'RELEASE_CERTIFICATE']) {
        expect(repo(AuditLog).insert).toHaveBeenCalledWith(
          expect.objectContaining({
            level: 'INFO',
            category: 'RELEASE',
            userId: caller.id,
            userEmail: caller.email,
            userRole: caller.roles.join(','),
            action,
            details: { programId: program.id, classCount: 2 },
          }),
        );
      }
    },
  );

  it('withdraws all transcripts and certificates, clears metadata, then can release again', async () => {
    markCertificateReleased();
    const result = await service.releaseTranscript(caller.id, program.id);
    expect(result).toMatchObject({
      isTranscriptReleased: false,
      isCertificateReleased: false,
      releasedAt: null,
      releasedBy: null,
    });
    for (const cls of classes)
      expect(cls).toMatchObject({
        isTranscriptReleased: false,
        transcriptReleasedAt: null,
        transcriptReleasedBy: null,
        isCertificateReleased: false,
        certificateReleasedAt: null,
        certificateReleasedBy: null,
      });
    expect(repo(AuditLog).insert).toHaveBeenCalledWith(
      expect.objectContaining({
        action: 'WITHDRAW_TRANSCRIPT',
        details: { programId: program.id, classCount: 2 },
      }),
    );
    await expect(
      service.releaseTranscript(caller.id, program.id),
    ).resolves.toMatchObject({
      isTranscriptReleased: true,
      isCertificateReleased: false,
      releasedBy: caller.id,
    });
  });

  it('withdraws certificates and clears only certificate metadata', async () => {
    markCertificateReleased();
    await expect(
      service.releaseCertificate(caller.id, program.id),
    ).resolves.toMatchObject({
      isTranscriptReleased: true,
      isCertificateReleased: false,
      releasedAt: null,
      releasedBy: null,
      incompleteStudents: [],
    });
    for (const cls of classes)
      expect(cls).toMatchObject({
        isTranscriptReleased: true,
        transcriptReleasedAt: previousRelease,
        transcriptReleasedBy: 'previous-head',
        isCertificateReleased: false,
        certificateReleasedAt: null,
        certificateReleasedBy: null,
      });
    expect(repo(AuditLog).insert).toHaveBeenCalledWith(
      expect.objectContaining({ action: 'WITHDRAW_CERTIFICATE' }),
    );
  });

  it.each(['transcript', 'certificate'] as const)(
    'releases mixed %s classes even when the first class is already released',
    async (kind) => {
      if (kind === 'certificate') markTranscriptReleased();
      classes[0][
        kind === 'transcript' ? 'isTranscriptReleased' : 'isCertificateReleased'
      ] = true;
      const result =
        kind === 'transcript'
          ? await service.releaseTranscript(caller.id, program.id)
          : await service.releaseCertificate(caller.id, program.id);
      expect(
        result[
          kind === 'transcript'
            ? 'isTranscriptReleased'
            : 'isCertificateReleased'
        ],
      ).toBe(true);
      expect(result.releasedBy).toBe(caller.id);
      expect(
        classes.every(
          (cls) =>
            cls[
              kind === 'transcript'
                ? 'isTranscriptReleased'
                : 'isCertificateReleased'
            ],
        ),
      ).toBe(true);
    },
  );

  it('rejects certificate release if any class transcript is unreleased, preserving the old 400 message', async () => {
    classes[0].isTranscriptReleased = true;
    await expect(
      service.releaseCertificate(caller.id, program.id),
    ).rejects.toEqual(
      new BadRequestException(
        'Gagal! Transkrip Nilai (Fase Micro) harus dirilis terlebih dahulu sebelum Sertifikat (Fase Massive) dapat dirilis.',
      ),
    );
    expect(repo(Class).save).not.toHaveBeenCalled();
    expect(repo(AuditLog).insert).not.toHaveBeenCalled();
  });

  it('reports incomplete Massive grades without blocking release and excludes staff enrollments', async () => {
    markTranscriptReleased();
    enrollments = [
      makeEnrollment(1),
      makeEnrollment(2),
      { ...makeEnrollment(3), roles: ['student', 'mentor'] },
    ];
    enrollments[0].competencyScores = [{ competencyId: 'coding', score: 80 }];
    const result = await service.releaseCertificate(caller.id, program.id);
    expect(result).toMatchObject({
      isCertificateReleased: true,
      incompleteStudents: [
        { id: 'student-1', name: 'Student 1', missingCount: 1 },
      ],
    });
  });

  it('uses weighted graded assignment fallback and imported scores for Massive completeness', async () => {
    markTranscriptReleased();
    enrollments = [makeEnrollment(1), makeEnrollment(2)];
    enrollments.forEach((e) => {
      e.competencyScores = [];
    });
    enrollments[1].externalScores = [
      { rubrikAssessmentId: 'massive', score: 0 },
    ];
    submissionQuery.getMany.mockResolvedValue([
      {
        studentId: 'student-1',
        score: 85,
        assignment: {
          competency: 'PROJECT',
          weight: 1,
          class: { batchId: batch.id },
        },
      },
    ]);
    await expect(
      service.releaseCertificate(caller.id, program.id),
    ).resolves.toMatchObject({ incompleteStudents: [] });
  });

  it('limits scope to active batches for the selected program and locks rows', async () => {
    await service.releaseTranscript(caller.id, program.id);
    expect(repo(Class).find).toHaveBeenCalledTimes(1);
    expect(repo(Class).find).toHaveBeenCalledWith({
      where: { programId: program.id, batchId: In([batch.id]) },
      order: { id: 'ASC' },
      lock: { mode: 'pessimistic_write' },
    });
  });

  it('falls back to all program classes when active batches have no matching classes', async () => {
    repo(Class).find.mockResolvedValueOnce([]).mockResolvedValueOnce(classes);
    await service.releaseTranscript(caller.id, program.id);
    expect(repo(Class).find).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ where: { programId: program.id } }),
    );
  });

  it('uses all program classes when there are no active batches', async () => {
    repo(Batch).find.mockResolvedValue([]);
    await service.releaseTranscript(caller.id, program.id);
    expect(repo(Class).find).toHaveBeenCalledTimes(1);
    expect(repo(Class).find).toHaveBeenCalledWith(
      expect.objectContaining({ where: { programId: program.id } }),
    );
  });

  it('returns 404 when no classes exist and does not write', async () => {
    repo(Class).find.mockResolvedValue([]);
    await expect(
      service.releaseTranscript(caller.id, program.id),
    ).rejects.toBeInstanceOf(NotFoundException);
    expect(repo(Class).save).not.toHaveBeenCalled();
  });

  it('propagates audit failures through the transaction instead of reporting success', async () => {
    repo(AuditLog).insert.mockRejectedValue(new Error('audit failed'));
    await expect(
      service.releaseTranscript(caller.id, program.id),
    ).rejects.toThrow('audit failed');
    expect(transaction).toHaveBeenCalledTimes(1);
  });

  it.each([
    {
      transcript: false,
      certificate: false,
      months: [1, 2],
      lock: 'TRANSCRIPT_NOT_RELEASED',
      certificateLock: 'CERTIFICATE_NOT_RELEASED',
    },
    {
      transcript: true,
      certificate: false,
      months: [],
      lock: 'LOGBOOK_MONTH1_INCOMPLETE',
      certificateLock: 'CERTIFICATE_NOT_RELEASED',
    },
    {
      transcript: true,
      certificate: true,
      months: [2],
      lock: 'LOGBOOK_MONTH1_INCOMPLETE',
      certificateLock: 'LOGBOOK_INCOMPLETE',
    },
  ])(
    'hides all grades when transcript is locked: $lock',
    async ({ transcript, certificate, months, lock, certificateLock }) => {
      classes[0].isTranscriptReleased = transcript;
      classes[0].isCertificateReleased = certificate;
      enrollments[0].logbooks = months.map((monthIndex) => ({
        monthIndex,
        status: LogbookStatus.ACCEPTED,
      }));
      const result = await service.findMyGrades('student-1');
      expect(result).toHaveLength(1);
      expect(result[0]).toMatchObject({
        locked: true,
        lockReason: lock,
        certificateLocked: true,
        certificateLockReason: certificateLock,
        microItems: [],
        massiveItems: [],
        totalMicroScore: 0,
        totalMassiveScore: 0,
        finalScore: 0,
        predicate: '',
        isTranscriptReleased: transcript,
        isCertificateReleased: certificate,
        program: { id: program.id },
        student: { id: 'student-1' },
        mentor: { name: 'Mentor' },
      });
      expect(enrollmentQuery.andWhere).toHaveBeenCalledWith(
        'student.id = :studentId',
        { studentId: 'student-1' },
      );
    },
  );

  it.each([
    { certificate: false, months: [1, 2], reason: 'CERTIFICATE_NOT_RELEASED' },
    { certificate: true, months: [1], reason: 'LOGBOOK_INCOMPLETE' },
  ])(
    'reveals only Micro grades when certificate is locked: $reason',
    async ({ certificate, months, reason }) => {
      markTranscriptReleased();
      classes[0].isCertificateReleased = certificate;
      enrollments[0].logbooks = months.map((monthIndex) => ({
        monthIndex,
        status: LogbookStatus.ACCEPTED,
      }));
      const [result] = await service.findMyGrades('student-1');
      expect(result).toMatchObject({
        locked: false,
        lockReason: null,
        certificateLocked: true,
        certificateLockReason: reason,
        totalMicroScore: 80,
        massiveItems: [],
        totalMassiveScore: 0,
        finalScore: 0,
        predicate: '',
      });
      expect(result.microItems).toHaveLength(1);
    },
  );

  it.each([null, '2001020029'])(
    'returns raw NIM %s without an email fallback and explicit null mentor identity',
    async (nim) => {
      repo(User).findOne.mockResolvedValue({
        id: 'student-1',
        name: 'Student',
        email: '2001020029@student.umrah.ac.id',
        nim,
      });
      const [result] = await service.findMyGrades('student-1');
      expect(result.student.nim).toBe(nim);
      expect(result.mentor).toEqual({
        name: 'Mentor',
        fullNameWithTitle: null,
        signatureUrl: null,
        email: 'mentor@example.test',
      });
    },
  );

  it('returns the personal mentor identity from the student class', async () => {
    Object.assign(classes[0].mentor, {
      fullNameWithTitle: 'Arifian Saputra, S. T.',
      signatureUrl: '/signatures/arifian-saputra.png',
    });
    const [result] = await service.findMyGrades('student-1');
    expect(result.mentor).toEqual({
      name: 'Mentor',
      fullNameWithTitle: 'Arifian Saputra, S. T.',
      signatureUrl: '/signatures/arifian-saputra.png',
      email: 'mentor@example.test',
    });
  });

  it('matches recap grades exactly for five students once both documents and logbooks are unlocked', async () => {
    markCertificateReleased();
    enrollments = Array.from({ length: 5 }, (_, index) =>
      makeEnrollment(index + 1),
    );
    enrollments[0].competencyScores = [
      { competencyId: 'coding', score: 65 },
      { competencyId: 'project', score: 95 },
    ];
    enrollments[1].externalScores = [
      { rubrikAssessmentId: 'massive', score: 99 },
    ];
    enrollments[2].competencyScores = [];
    enrollments[3].competencyScores[0].score = 0;
    enrollments[4].competencyScores = [];
    submissionQuery.getMany.mockResolvedValue([
      {
        studentId: 'student-5',
        score: 87,
        assignment: {
          competency: 'Coding',
          weight: 1,
          class: { batchId: batch.id },
        },
      },
    ]);
    const recap = await service.findProgramRecap(program.id, {
      id: caller.id,
      roles: ['admin'],
    });
    for (const student of recap.students) {
      repo(User).findOne.mockResolvedValue({
        id: student.id,
        name: student.name,
        email: `${student.id}@example.test`,
      });
      const [grades] = await service.findMyGrades(student.id);
      expect(grades).toMatchObject({
        locked: false,
        lockReason: null,
        certificateLocked: false,
        certificateLockReason: null,
        microItems: student.microItems,
        massiveItems: student.massiveItems,
        totalMicroScore: student.totalMicroScore,
        totalMassiveScore: student.totalMassiveScore,
        finalScore: student.finalScore,
        predicate: student.predicate,
        logbookCompletionStatus: student.logbookCompletionStatus,
      });
    }
  });
});
