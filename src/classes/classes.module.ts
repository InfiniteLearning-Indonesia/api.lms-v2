import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { ClassesService } from './classes.service';
import { ClassesController } from './classes.controller';
import { Program } from './entities/program.entity';
import { Batch } from './entities/batch.entity';
import { Class } from './entities/class.entity';
import { Enrollment } from './entities/enrollment.entity';
import { Material } from './entities/material.entity';
import { Assignment } from './entities/assignment.entity';
import { Competency } from './entities/competency.entity';
import { Submission } from './entities/submission.entity';
import { Logbook } from './entities/logbook.entity';
import { User } from '../users/entities/user.entity';
import { UsersModule } from '../users/users.module';

@Module({
  imports: [
    TypeOrmModule.forFeature([Program, Batch, Class, Enrollment, Material, Assignment, Competency, Submission, Logbook, User]),
    UsersModule,
  ],
  controllers: [ClassesController],
  providers: [ClassesService],
})
export class ClassesModule {}

