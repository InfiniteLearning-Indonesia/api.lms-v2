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
import { Batch } from '../../classes/entities/batch.entity';

export enum AttendanceStatus {
  HADIR_ON_CAM = 'Hadir On-Cam',
  HADIR_OFF_CAM = 'Hadir Off-cam',
  IZIN_SAKIT = 'Izin/Sakit',
  ALPHA = 'Alpha',
}

@Entity('attendances')
export class Attendance {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  studentId: string;

  @ManyToOne(() => User)
  @JoinColumn({ name: 'studentId' })
  student: User;

  @Column()
  batchId: string;

  @ManyToOne(() => Batch)
  @JoinColumn({ name: 'batchId' })
  batch: Batch;

  @Column({ type: 'date' })
  date: Date;

  @Column({ type: 'enum', enum: AttendanceStatus })
  status: AttendanceStatus;

  @Column({ type: 'int', default: 0 })
  spLevel: number; // 0 = no SP, 1 = SP1, 2 = SP2, 3 = SP3, 4 = Suspended

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
