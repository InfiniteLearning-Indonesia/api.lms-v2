import { BadRequestException } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { plainToInstance } from 'class-transformer';
import { validate } from 'class-validator';
import { EntityManager, In } from 'typeorm';
import { ClassesService } from './classes.service';
import { BulkUpsertCompetencyScoresDto } from './dto/bulk-upsert-competency-scores.dto';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity';
import { Material } from './entities/material.entity';
import { Assignment } from './entities/assignment.entity';
import { Competency } from './entities/competency.entity';
import { RubrikAssessment } from './entities/rubrik-assessment.entity';
import { RubrikAssessmentScore } from './entities/rubrik-assessment-score.entity';
import { Program } from './entities/program.entity';
import { Batch } from './entities/batch.entity';
import { User } from '../users/entities/user.entity';
import { Submission } from './entities/submission.entity';
import { Logbook } from './entities/logbook.entity';
import { MentorAsyncDay } from './entities/mentor-async-day.entity';
import { ProgramCompetency } from './entities/program-competency.entity';
import { CompetencyScore } from './entities/competency-score.entity';
import { AiEvaluatorService } from './services/ai-evaluator.service';

const studentId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const competencyId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
const otherCompetencyId = 'cccccccc-cccc-4ccc-8ccc-cccccccccccc';
const missingId = 'dddddddd-dddd-4ddd-8ddd-dddddddddddd';
const score = { studentId, competencyId, score: 80 };

describe('ClassesService.bulkUpsertCompetencyScores', () => {
  let service: ClassesService;
  let find: jest.Mock;
  let upsert: jest.Mock;
  let transaction: jest.Mock;

  beforeEach(async () => {
    find = jest
      .fn()
      .mockImplementation((entity: unknown) =>
        Promise.resolve(
          entity === User
            ? [{ id: studentId }]
            : [{ id: competencyId }, { id: otherCompetencyId }],
        ),
      );
    upsert = jest.fn().mockResolvedValue({});
    const manager = { find, upsert } as unknown as EntityManager;
    transaction = jest
      .fn()
      .mockImplementation(
        (work: (manager: EntityManager) => Promise<unknown>) => work(manager),
      );
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
    const module = await Test.createTestingModule({
      providers: [
        ClassesService,
        ...entities.map((entity) => ({
          provide: getRepositoryToken(entity),
          useValue:
            entity === CompetencyScore
              ? { manager: { dataSource: { transaction } } }
              : {},
        })),
        { provide: AiEvaluatorService, useValue: {} },
      ],
    }).compile();
    service = module.get(ClassesService);
  });

  it('saves multiple items with one upsert inside one transaction', async () => {
    const scores = [
      score,
      { ...score, competencyId: otherCompetencyId, score: 95 },
    ];

    await expect(
      service.bulkUpsertCompetencyScores({ scores }),
    ).resolves.toEqual({ saved: 2, skipped: 0 });

    expect(transaction).toHaveBeenCalledTimes(1);
    expect(find).toHaveBeenCalledTimes(2);
    expect(find).toHaveBeenCalledWith(User, {
      select: { id: true },
      where: { id: In([studentId]) },
    });
    expect(find).toHaveBeenCalledWith(Competency, {
      select: { id: true },
      where: { id: In([competencyId, otherCompetencyId]) },
    });
    expect(upsert).toHaveBeenCalledTimes(1);
    expect(upsert).toHaveBeenCalledWith(
      CompetencyScore,
      scores.map((item) => ({ ...item, updatedAt: expect.any(Date) as Date })),
      ['studentId', 'competencyId'],
    );
  });

  it('accepts the 5000-item limit', async () => {
    const scores = Array.from({ length: 5000 }, (_, index) => ({
      ...score,
      competencyId: `cccccccc-cccc-4ccc-8ccc-${index.toString(16).padStart(12, '0')}`,
    }));
    find
      .mockResolvedValueOnce([{ id: studentId }])
      .mockResolvedValueOnce(scores.map((item) => ({ id: item.competencyId })));

    await expect(
      service.bulkUpsertCompetencyScores({ scores }),
    ).resolves.toEqual({ saved: 5000, skipped: 0 });
    expect(upsert).toHaveBeenCalledTimes(1);
    expect(upsert).toHaveBeenCalledWith(
      CompetencyScore,
      scores.map((item) => ({ ...item, updatedAt: expect.any(Date) as Date })),
      ['studentId', 'competencyId'],
    );
  });

  it('rejects more than 5000 items before accessing the database', async () => {
    const scores = Array.from({ length: 5001 }, () => ({ ...score }));

    await expect(
      service.bulkUpsertCompetencyScores({ scores }),
    ).rejects.toEqual(
      new BadRequestException('Maksimal 5000 nilai per permintaan.'),
    );
    expect(transaction).not.toHaveBeenCalled();
  });

  it('skips missing students and competencies without failing valid items', async () => {
    await expect(
      service.bulkUpsertCompetencyScores({
        scores: [
          score,
          { ...score, studentId: missingId },
          { ...score, competencyId: missingId },
        ],
      }),
    ).resolves.toEqual({ saved: 1, skipped: 2 });
    expect(upsert).toHaveBeenCalledWith(
      CompetencyScore,
      [{ ...score, updatedAt: expect.any(Date) as Date }],
      ['studentId', 'competencyId'],
    );
  });

  it('skips an entirely missing batch without issuing an empty upsert', async () => {
    await expect(
      service.bulkUpsertCompetencyScores({
        scores: [{ ...score, studentId: missingId }],
      }),
    ).resolves.toEqual({ saved: 0, skipped: 1 });
    expect(upsert).not.toHaveBeenCalled();
  });

  it('returns zero counts for an empty array without accessing the database', async () => {
    await expect(
      service.bulkUpsertCompetencyScores({ scores: [] }),
    ).resolves.toEqual({ saved: 0, skipped: 0 });
    expect(transaction).not.toHaveBeenCalled();
  });

  it('keeps the last score for duplicate keys and accepts uppercase UUIDs', async () => {
    await expect(
      service.bulkUpsertCompetencyScores({
        scores: [
          score,
          {
            studentId: studentId.toUpperCase(),
            competencyId: competencyId.toUpperCase(),
            score: 100,
          },
        ],
      }),
    ).resolves.toEqual({ saved: 2, skipped: 0 });
    expect(upsert).toHaveBeenCalledWith(
      CompetencyScore,
      [{ ...score, score: 100, updatedAt: expect.any(Date) as Date }],
      ['studentId', 'competencyId'],
    );
  });

  it('propagates database errors so the transaction can roll back', async () => {
    const error = new Error('Database write failed');
    upsert.mockRejectedValue(error);

    await expect(
      service.bulkUpsertCompetencyScores({ scores: [score] }),
    ).rejects.toBe(error);
  });
});

describe('BulkUpsertCompetencyScoresDto', () => {
  it('accepts valid nested items including score boundaries', async () => {
    const dto = plainToInstance(BulkUpsertCompetencyScoresDto, {
      scores: [{ ...score, score: 0 }, { ...score, score: 100 }, score],
    });
    await expect(validate(dto)).resolves.toEqual([]);
  });

  it.each([
    {},
    { scores: [] },
    { scores: {} },
    { scores: [{ ...score, studentId: 'invalid' }] },
    { scores: [{ ...score, competencyId: 'invalid' }] },
    { scores: [{ ...score, score: -1 }] },
    { scores: [{ ...score, score: 101 }] },
    { scores: [{ ...score, score: '80' }] },
  ])('rejects an invalid payload: %j', async (payload) => {
    const dto = plainToInstance(BulkUpsertCompetencyScoresDto, payload);
    expect(await validate(dto)).not.toHaveLength(0);
  });
});
