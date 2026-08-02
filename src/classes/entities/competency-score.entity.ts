import {
  Entity,
  PrimaryGeneratedColumn,
  Column,
  CreateDateColumn,
  UpdateDateColumn,
  ManyToOne,
  JoinColumn,
  Unique,
} from 'typeorm';
import { User } from '../../users/entities/user.entity.js';
import { Competency } from './competency.entity.js';

@Entity('competency_scores')
@Unique(['studentId', 'competencyId'])
export class CompetencyScore {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  studentId: string;

  @Column()
  competencyId: string;

  @Column({ type: 'float' })
  score: number;

  @ManyToOne(() => User, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'studentId' })
  student: User;

  @ManyToOne(() => Competency, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'competencyId' })
  competency: Competency;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
