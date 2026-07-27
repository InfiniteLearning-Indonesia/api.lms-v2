import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn, OneToMany } from 'typeorm';
import { Class } from './class.entity';
import { Submission } from './submission.entity';

@Entity('assignments')
export class Assignment {
    @PrimaryGeneratedColumn('uuid')
    id: string;

    @Column()
    classId: string;

    @Column()
    title: string;

    @Column({ nullable: true })
    competency: string;

    @Column({ type: 'jsonb', nullable: true })
    selectedRubrics: any;

    @Column({ type: 'text', nullable: true })
    description: string;

    @Column({ type: 'timestamp', nullable: true })
    dueDate: Date;

    @Column({ type: 'float', default: 0.1 })
    weight: number;

    @ManyToOne(() => Class, (cls) => cls.assignments)
    @JoinColumn({ name: 'classId' })
    class: Class;

    @OneToMany(() => Submission, (sub) => sub.assignment)
    submissions: Submission[];

    @CreateDateColumn()
    createdAt: Date;

    @UpdateDateColumn()
    updatedAt: Date;
}
