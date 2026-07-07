import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn } from 'typeorm';
import { Program } from './program.entity.js';
import { User } from '../../users/entities/user.entity.js';

@Entity('competencies')
export class Competency {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  programId: string;

  @Column({ nullable: true })
  creatorMentorId: string | null;

  @Column()
  name: string;

  @Column({ type: 'text', nullable: true })
  description: string;

  @Column({ type: 'varchar', default: 'Technical' }) // Technical, Soft Skills (CCA), Capstone Project
  category: string;

  @ManyToOne(() => Program)
  @JoinColumn({ name: 'programId' })
  program: Program;

  @ManyToOne(() => User, { onDelete: 'SET NULL' })
  @JoinColumn({ name: 'creatorMentorId' })
  creatorMentor: User;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
