import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn } from 'typeorm';
import { Class } from './class.entity';

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

    @Column({ type: 'text', nullable: true })
    description: string;

    @Column({ type: 'timestamp', nullable: true })
    dueDate: Date;

    @Column({ type: 'jsonb', nullable: true })
    rubric: any;

    @ManyToOne(() => Class, (cls) => cls.assignments)
    @JoinColumn({ name: 'classId' })
    class: Class;

    @CreateDateColumn()
    createdAt: Date;

    @UpdateDateColumn()
    updatedAt: Date;
}
