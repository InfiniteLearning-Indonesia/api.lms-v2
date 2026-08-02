import {
  Entity,
  PrimaryGeneratedColumn,
  Column,
  CreateDateColumn,
  UpdateDateColumn,
  ManyToOne,
  JoinColumn,
} from 'typeorm';
import { User } from '../../users/entities/user.entity';
import { Batch } from './batch.entity';

export enum LogbookStatus {
  PENDING = 'pending',
  REVISION = 'revision',
  ACCEPTED = 'accepted',
}

@Entity('logbooks')
export class Logbook {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  studentId: string;

  @Column()
  batchId: string;

  @Column({ type: 'int' })
  monthIndex: number;

  @Column({ type: 'text' })
  q1_experience: string;

  @Column({ type: 'text' })
  q2_progress: string;

  @Column({ type: 'text' })
  q3_challenges: string;

  @Column({ type: 'text' })
  q4_competencies: string;

  @Column({ type: 'enum', enum: LogbookStatus, default: LogbookStatus.PENDING })
  status: LogbookStatus;

  @Column({ type: 'text', nullable: true })
  mentorFeedback?: string | null;

  @ManyToOne(() => User, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'studentId' })
  student: User;

  @ManyToOne(() => Batch, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'batchId' })
  batch: Batch;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
