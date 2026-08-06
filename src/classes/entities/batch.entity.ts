import {
  Entity,
  PrimaryGeneratedColumn,
  Column,
  CreateDateColumn,
  UpdateDateColumn,
} from 'typeorm';

export enum BatchStatus {
  DRAFT = 'draft',
  ACTIVE = 'active',
  COMPLETED = 'completed',
}

@Entity('batches')
export class Batch {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  name: string;

  @Column({ type: 'enum', enum: BatchStatus, default: BatchStatus.ACTIVE })
  status: BatchStatus; // draft, active, completed

  @Column({ nullable: true })
  programId?: string;

  @Column('simple-array', { nullable: true })
  includedProgramIds?: string[];

  @Column({ type: 'timestamp', nullable: true })
  startDate?: Date | null;

  @Column({ type: 'timestamp', nullable: true })
  endDate?: Date | null;

  @Column({ type: 'timestamp', nullable: true })
  microStartDate?: Date | null;

  @Column({ type: 'timestamp', nullable: true })
  microEndDate?: Date | null;

  @Column({ type: 'timestamp', nullable: true })
  massiveStartDate?: Date | null;

  @Column({ type: 'timestamp', nullable: true })
  massiveEndDate?: Date | null;

  @Column({ type: 'jsonb', nullable: true })
  logbookSchedule?: {
    monthIndex: number;
    startDate: string; // ISO date string
    endDate?: string; // ISO date string (optional)
    label?: string; // e.g., "Spesial Closing"
  }[];

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
