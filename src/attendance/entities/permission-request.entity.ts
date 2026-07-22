import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn } from 'typeorm';
import { User } from '../../users/entities/user.entity';
import { Batch } from '../../classes/entities/batch.entity';

@Entity('permission_requests')
export class PermissionRequest {
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
  date: string; // Format YYYY-MM-DD

  @Column()
  category: string; // 'Izin' or 'Sakit'

  @Column({ type: 'text' })
  reason: string;

  @Column({ type: 'simple-json', nullable: true })
  proofFiles: string[]; // Base64 or URLs (images and PDF)

  @Column({ type: 'simple-json', nullable: true })
  mentorChatFiles: string[]; // Base64 or URLs (images only)

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
