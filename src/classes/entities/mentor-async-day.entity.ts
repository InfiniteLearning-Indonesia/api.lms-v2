import { Entity, PrimaryGeneratedColumn, Column, CreateDateColumn, UpdateDateColumn, ManyToOne, JoinColumn } from 'typeorm';
import { User } from '../../users/entities/user.entity';

@Entity('mentor_async_days')
export class MentorAsyncDay {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  mentorId: string;

  @Column({ type: 'varchar' })
  date: string; // YYYY-MM-DD

  @Column({ type: 'text', nullable: true })
  note: string;

  @ManyToOne(() => User, { onDelete: 'CASCADE' })
  @JoinColumn({ name: 'mentorId' })
  mentor: User;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
