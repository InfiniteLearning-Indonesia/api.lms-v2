import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, OneToMany, JoinColumn } from 'typeorm';
import { Program } from './program.entity';
import { Batch } from './batch.entity';
import { User } from '../../users/entities/user.entity';
import { Material } from './material.entity';
import { Assignment } from './assignment.entity';

@Entity('classes')
export class Class {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  programId: string;

  @Column()
  batchId: string;

  @Column({ type: 'varchar', nullable: true })
  mentorId?: string | null;

  @Column({ type: 'boolean', default: false })
  isTranscriptReleased: boolean;

  @Column({ type: 'boolean', default: false })
  isCertificateReleased: boolean;

  @Column({ type: 'jsonb', nullable: true, default: [] })
  importantLinks?: { id: string; title: string; url: string; iconType?: string }[];

  @ManyToOne(() => Program)
  @JoinColumn({ name: 'programId' })
  program: Program;

  @ManyToOne(() => Batch)
  @JoinColumn({ name: 'batchId' })
  batch: Batch;

  @ManyToOne(() => User, { onDelete: 'SET NULL' })
  @JoinColumn({ name: 'mentorId' })
  mentor: User;

  @OneToMany(() => Material, (material) => material.class)
  materials: Material[];

  @OneToMany(() => Assignment, (assignment) => assignment.class)
  assignments: Assignment[];

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
