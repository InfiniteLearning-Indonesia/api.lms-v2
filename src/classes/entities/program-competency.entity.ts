import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn, OneToMany } from 'typeorm';
import { Program } from './program.entity.js';
import { Competency } from './competency.entity.js';

@Entity('program_competencies')
export class ProgramCompetency {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column({ nullable: true })
  programId: string | null;

  @Column()
  name: string;

  @Column({ type: 'text', nullable: true })
  description: string;

  @Column({ type: 'varchar', default: 'Technical' }) // Technical, Soft Skills (CCA), Capstone Project
  category: string;

  @ManyToOne(() => Program)
  @JoinColumn({ name: 'programId' })
  program: Program;

  @OneToMany(() => Competency, (comp) => comp.programCompetency)
  syllabuses: Competency[];

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
