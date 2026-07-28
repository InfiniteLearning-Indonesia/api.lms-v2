import { NestFactory } from '@nestjs/core';
import { AppModule } from './src/app.module';
import { getRepositoryToken } from '@nestjs/typeorm';
import { ProgramCompetency } from './src/classes/entities/program-competency.entity';
import { RubrikAssessment } from './src/classes/entities/rubrik-assessment.entity';
import { Repository } from 'typeorm';

async function bootstrap() {
  const app = await NestFactory.createApplicationContext(AppModule);
  
  const pcRepo = app.get<Repository<ProgramCompetency>>(getRepositoryToken(ProgramCompetency));
  const raRepo = app.get<Repository<RubrikAssessment>>(getRepositoryToken(RubrikAssessment));

  const pcs = await pcRepo.find();
  
  for (const pc of pcs) {
    const initialExists = await raRepo.findOne({ where: { programCompetencyId: pc.id, phase: 'Micro' }});
    if (!initialExists) {
      await raRepo.save({
        name: pc.name,
        phase: 'Micro',
        programId: pc.programId || null,
        programCompetencyId: pc.id,
        isGlobal: !pc.programId,
        competencies: [],
        subAssessments: []
      });
      console.log(`Created Initial phase RA for PC: ${pc.name}`);
    }

    const finalExists = await raRepo.findOne({ where: { programCompetencyId: pc.id, phase: 'Massive' }});
    if (!finalExists) {
      await raRepo.save({
        name: pc.name,
        phase: 'Massive',
        programId: pc.programId || null,
        programCompetencyId: pc.id,
        isGlobal: !pc.programId,
        competencies: [],
        subAssessments: []
      });
      console.log(`Created Final phase RA for PC: ${pc.name}`);
    }
  }

  console.log("Migration complete!");
  await app.close();
}

bootstrap();
