import {
  Entity,
  Column,
  PrimaryGeneratedColumn,
  CreateDateColumn,
} from 'typeorm';

@Entity('audit_logs')
export class AuditLog {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  timestampWib: string;

  @CreateDateColumn()
  createdAt: Date;

  @Column({ type: 'varchar', length: 10, default: 'INFO' })
  level: string; // INFO, WARN, ERROR

  @Column({ type: 'varchar', length: 20 })
  category: string; // SECURITY, AUTH, MUTATION

  @Column({ nullable: true })
  userId: string;

  @Column({ nullable: true })
  userEmail: string;

  @Column({ nullable: true })
  userRole: string;

  @Column({ nullable: true })
  ipAddress: string;

  @Column()
  action: string;

  @Column({ nullable: true })
  method: string;

  @Column({ nullable: true })
  path: string;

  @Column({ type: 'int', nullable: true })
  statusCode: number;

  @Column({ type: 'json', nullable: true })
  details: any;
}
