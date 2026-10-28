import { ForbiddenException, NotFoundException } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { In } from 'typeorm';
import { ClassesService } from './classes.service';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity';
import { Material } from './entities/material.entity';
import { Assignment } from './entities/assignment.entity';
import { Competency } from './entities/competency.entity';
import { RubrikAssessment } from './entities/rubrik-assessment.entity';
import { RubrikAssessmentScore } from './entities/rubrik-assessment-score.entity';
import { Program } from './entities/program.entity';
import { Batch, BatchStatus } from './entities/batch.entity';
import { User } from '../users/entities/user.entity';
import { Submission } from './entities/submission.entity';
import { Logbook } from './entities/logbook.entity';
import { MentorAsyncDay } from './entities/mentor-async-day.entity';
import { ProgramCompetency } from './entities/program-competency.entity';
import { CompetencyScore } from './entities/competency-score.entity';
import { AiEvaluatorService } from './services/ai-evaluator.service';

function mockRepository() {
  return {
    find: jest.fn<Promise<unknown[]>, [unknown?]>().mockResolvedValue([]),
    findOne: jest.fn<Promise<unknown>, [unknown?]>().mockResolvedValue(null),
    exists: jest.fn<Promise<boolean>, [unknown?]>().mockResolvedValue(false),
  };
}

describe('ClassesService.findBatchRecapClasses', () => {
  const batch = { id: 'batch-1', name: 'Batch 1', status: BatchStatus.ACTIVE };
  const program = { id: 'program-1', name: 'Web' };
  const classes = [
    {
      id: 'class-1',
      programId: program.id,
      batchId: batch.id,
      mentorId: 'mentor-1',
      program,
      batch,
    },
    {
      id: 'class-2',
      programId: program.id,
      batchId: batch.id,
      mentorId: 'mentor-2',
      program,
      batch,
    },
  ];
  let service: ClassesService;
  let batchRepository: ReturnType<typeof mockRepository>;
  let classRepository: ReturnType<typeof mockRepository>;
  let enrollmentRepository: ReturnType<typeof mockRepository>;
  let repositories: ReturnType<typeof mockRepository>[];
  let healEnrollments: jest.SpyInstance;

  beforeEach(async () => {
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
    ];
    repositories = entities.map(() => mockRepository());
    const module = await Test.createTestingModule({
      providers: [
        ClassesService,
        ...entities.map((entity, index) => ({
          provide: getRepositoryToken(entity),
          useValue: repositories[index],
        })),
        { provide: AiEvaluatorService, useValue: {} },
      ],
    }).compile();
    service = module.get(ClassesService);
    batchRepository = module.get(getRepositoryToken(Batch));
    classRepository = module.get(getRepositoryToken(Class));
    enrollmentRepository = module.get(getRepositoryToken(Enrollment));
    batchRepository.findOne.mockResolvedValue(batch);
    classRepository.find.mockResolvedValue(classes);
    classRepository.exists.mockResolvedValue(true);
    healEnrollments = jest
      .spyOn(service, 'healEnrollments')
      .mockRejectedValue(new Error('Recap must not heal enrollments'));
  });

  afterEach(() => {
    expect(healEnrollments).not.toHaveBeenCalled();
    for (const repository of repositories) {
      if (
        repository !== batchRepository &&
        repository !== classRepository &&
        repository !== enrollmentRepository
      ) {
        expect(repository.find).not.toHaveBeenCalled();
        expect(repository.findOne).not.toHaveBeenCalled();
        expect(repository.exists).not.toHaveBeenCalled();
      }
    }
  });

  it.each(['admin', 'facilitator'])(
    'returns all batch classes for %s without checking mentor membership',
    async (role) => {
      const result = await service.findBatchRecapClasses(batch.id, {
        id: 'staff-1',
        roles: [role],
      });

      expect(result).toEqual(
        classes.map((cls) => ({
          id: cls.id,
          programId: cls.programId,
          batchId: cls.batchId,
          program,
          batch,
          enrolledStudents: [],
        })),
      );
      expect(classRepository.exists).not.toHaveBeenCalled();
      expect(classRepository.find).toHaveBeenCalledWith({
        where: { batchId: batch.id },
        relations: { program: true, batch: true },
        order: { createdAt: 'ASC' },
      });
    },
  );

  it('returns all batch classes for a professional mentor teaching in the batch', async () => {
    const result = await service.findBatchRecapClasses(batch.id, {
      id: 'mentor-1',
      roles: ['mentor'],
      specialization: 'Professional',
    });

    expect(result.map((cls) => cls.id)).toEqual(['class-1', 'class-2']);
  });

  it('rejects a mentor without a class in the batch', async () => {
    classRepository.exists.mockResolvedValue(false);

    await expect(
      service.findBatchRecapClasses(batch.id, {
        id: 'other-mentor',
        roles: ['mentor'],
      }),
    ).rejects.toEqual(
      new ForbiddenException('Kamu tidak mengajar di batch ini.'),
    );
    expect(enrollmentRepository.find).not.toHaveBeenCalled();
  });

  it.each(['admin', 'mentor'])(
    'rejects a missing batch before checking authorization for %s',
    async (role) => {
      batchRepository.findOne.mockResolvedValue(null);

      await expect(
        service.findBatchRecapClasses('missing-batch', {
          id: 'caller-1',
          roles: [role],
        }),
      ).rejects.toEqual(new NotFoundException('Batch tidak ditemukan.'));
      expect(classRepository.exists).not.toHaveBeenCalled();
      expect(classRepository.find).not.toHaveBeenCalled();
      expect(enrollmentRepository.find).not.toHaveBeenCalled();
    },
  );

  it('includes only students, groups enrollments by class, sorts names, and returns lightweight data', async () => {
    const zara = {
      id: 'student-1',
      name: 'Zara',
      email: 'zara@example.test',
      avatarUrl: null,
      status: 'active',
      roles: ['STUDENT'],
      password: 'must-not-be-returned',
    };
    const adi = {
      id: 'student-2',
      name: 'Adi',
      email: 'adi@example.test',
      avatarUrl: '/adi.png',
      status: 'suspended',
      role: 'STUDENT',
    };
    enrollmentRepository.find.mockResolvedValue([
      { classId: 'class-1', student: zara },
      { classId: 'class-1', student: adi },
      { classId: 'class-2', student: zara },
      { classId: 'class-1', student: null },
      { classId: 'class-1', student: { id: 'unknown', roles: [] } },
      ...['admin', 'mentor', 'facilitator'].flatMap((role) => [
        { classId: 'class-1', student: { id: role, roles: [role] } },
        { classId: 'class-1', student: { id: `legacy-${role}`, role } },
      ]),
    ]);

    const result = await service.findBatchRecapClasses(batch.id, {
      roles: ['admin'],
    });

    const projectStudent = (student: typeof adi | typeof zara) => ({
      id: student.id,
      name: student.name,
      email: student.email,
      avatarUrl: student.avatarUrl,
      status: student.status,
    });
    expect(result[0].enrolledStudents).toEqual([
      projectStudent(adi),
      projectStudent(zara),
    ]);
    expect(result[1].enrolledStudents).toEqual([projectStudent(zara)]);
    expect(enrollmentRepository.find).toHaveBeenCalledTimes(1);
    expect(enrollmentRepository.find).toHaveBeenCalledWith({
      where: { classId: In(['class-1', 'class-2']) },
      relations: { student: true },
    });
  });

  it('returns an empty array for a batch without classes without fetching enrollments', async () => {
    classRepository.find.mockResolvedValue([]);

    await expect(
      service.findBatchRecapClasses(batch.id, { roles: ['admin'] }),
    ).resolves.toEqual([]);
    expect(enrollmentRepository.find).not.toHaveBeenCalled();
  });
});
