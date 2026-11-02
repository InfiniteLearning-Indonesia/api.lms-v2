import { Test } from '@nestjs/testing';
import { getRepositoryToken } from '@nestjs/typeorm';
import { ClassesService } from './classes.service';
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

const relations = {
  program: true,
  creatorMentor: true,
  programCompetency: true,
};

describe('ClassesService.getCompetencies', () => {
  let service: ClassesService;
  const competencies = [
    { id: 'research', name: 'Product Research', programId: 'other-program' },
    { id: 'global', name: 'Teamwork', isGlobal: true },
  ];
  const find = jest.fn();

  beforeEach(async () => {
    find.mockReset().mockResolvedValue(competencies);
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
          useValue: entity === Competency ? { find } : {},
        })),
        { provide: AiEvaluatorService, useValue: {} },
      ],
    }).compile();
    service = module.get(ClassesService);
  });

  it('returns all competencies without a program filter for scope=all', async () => {
    await expect(
      service.getCompetencies('active-program', 'all'),
    ).resolves.toEqual(competencies);
    expect(find).toHaveBeenCalledWith({ relations, order: { name: 'ASC' } });
  });

  it('preserves the active program and global filter without scope', async () => {
    await service.getCompetencies('active-program');
    expect(find).toHaveBeenCalledWith({
      where: [{ programId: 'active-program' }, { isGlobal: true }],
      relations,
    });
  });

  it.each([undefined, ''])(
    'returns all for scope=all with programId=%s',
    async (programId) => {
      await expect(service.getCompetencies(programId, 'all')).resolves.toEqual(
        competencies,
      );
      expect(find).toHaveBeenCalledWith({ relations, order: { name: 'ASC' } });
    },
  );

  it('ignores unsupported scopes', async () => {
    await service.getCompetencies('active-program', 'unsupported');
    expect(find).toHaveBeenCalledWith({
      where: [{ programId: 'active-program' }, { isGlobal: true }],
      relations,
    });
  });

  it('preserves the default query without a program', async () => {
    await service.getCompetencies();
    expect(find).toHaveBeenCalledWith({ where: {}, relations });
  });
});
