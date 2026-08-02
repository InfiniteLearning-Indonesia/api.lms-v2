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
import { RubrikAssessment } from './rubrik-assessment.entity.js';

@Entity('rubrik_assessment_scores')
@Unique(['studentId', 'rubrikAssessmentId'])
export class RubrikAssessmentScore {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  studentId: string;

  @Column()
  rubrikAssessmentId: string;

  @Column({ type: 'float' })
  score: number;

  @ManyToOne(() => User, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'studentId' })
  student: User;

  @ManyToOne(() => RubrikAssessment, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'rubrikAssessmentId' })
  rubrikAssessment: RubrikAssessment;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
