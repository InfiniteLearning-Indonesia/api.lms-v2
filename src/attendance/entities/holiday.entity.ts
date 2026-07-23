import { Entity, PrimaryColumn, Column, CreateDateColumn, UpdateDateColumn } from 'typeorm';

@Entity('holidays')
export class Holiday {
  @PrimaryColumn({ type: 'date' })
  date: Date; // Primary key, as there's usually 1 holiday (or we just map by date)

  @Column()
  name: string;

  @Column({ nullable: true })
  type: string;

  @Column({ type: 'int', default: 1 })
  isActive: number;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
