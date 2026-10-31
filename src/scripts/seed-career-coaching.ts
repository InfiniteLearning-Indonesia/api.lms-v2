import { DataSource, ILike, IsNull } from 'typeorm';
import { Competency } from '../classes/entities/competency.entity.js';
import { RubrikAssessment } from '../classes/entities/rubrik-assessment.entity.js';
import { runSeed } from './seed-data-source.js';

// Run manually after build: node dist/src/scripts/seed-career-coaching.js
export async function seedCareerCoaching(dataSource: DataSource) {
  await dataSource.transaction(async (manager) => {
    const competencies = manager.getRepository(Competency);
    const rubrics = manager.getRepository(RubrikAssessment);
    const where = {
      name: ILike('Career Coaching'),
      isGlobal: true,
      programId: IsNull(),
    };
    let competency = await competencies.findOne({ where });
    if (!competency) {
      competency = await competencies.save(
        competencies.create({
          name: 'Career Coaching',
          isGlobal: true,
          programId: null,
          category: 'Technical',
        }),
      );
    }
    const rubric = await rubrics.findOne({
      where: { ...where, phase: 'Micro' },
    });
    if (!rubric) {
      await rubrics.save(
        rubrics.create({
          name: 'Career Coaching',
          isGlobal: true,
          programId: null,
          phase: 'Micro',
          competencies: [{ competencyId: competency.id, weight: 1 }],
        }),
      );
    }
  });
}

if (require.main === module) {
  runSeed(seedCareerCoaching).catch((error: unknown) => {
    console.error(error);
    process.exitCode = 1;
  });
}
