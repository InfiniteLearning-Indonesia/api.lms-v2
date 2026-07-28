import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, ManyToOne, JoinColumn } from 'typeorm';
import { Program } from './program.entity.js';
import { User } from '../../users/entities/user.entity.js';

@Entity('rubrik_assessments')
export class RubrikAssessment {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column({ nullable: true })
  programId: string | null;

  @Column({ type: 'boolean', default: false })
  isGlobal: boolean;

  @Column({ nullable: true })
  creatorMentorId: string | null;

  @Column()
  name: string;

  @Column({ type: 'text', nullable: true })
  description: string;

  @Column({ type: 'varchar', default: 'Micro' }) // Micro, Massive
  phase: string;

  @Column({ type: 'jsonb', nullable: true, default: [] })
  competencies: any; // array of { competencyId: string, weight: number }

  @Column({ type: 'jsonb', nullable: true, default: [] })
  subAssessments: any; // array of { assessmentId: string, weight: number }

  @ManyToOne(() => Program)
  @JoinColumn({ name: 'programId' })
  program: Program;

  @Column({ nullable: true })
  programCompetencyId: string | null;

  @ManyToOne('ProgramCompetency', { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'programCompetencyId' })
  programCompetency: any;

  @ManyToOne(() => User, { onDelete: 'SET NULL' })
  @JoinColumn({ name: 'creatorMentorId' })
  creatorMentor: User;

  @CreateDateColumn()
  createdAt: Date;
}
