import {
  Entity,
  Column,
  PrimaryGeneratedColumn,
  CreateDateColumn,
  UpdateDateColumn,
} from 'typeorm';

export enum UserRole {
  ADMIN = 'admin',
  FACILITATOR = 'facilitator',
  MENTOR = 'mentor',
  STUDENT = 'student',
}

export enum UserStatus {
  INVITED = 'invited',
  ACTIVE = 'active',
  SUSPENDED = 'suspended',
  INACTIVE = 'inactive',
  GRADUATED = 'graduated',
}

export enum MentorSpecialization {
  AI = 'AI',
  WEB = 'Web',
  MOBILE = 'Mobile',
  GAME = 'Game',
  UIUX = 'UI/UX',
  PROFESSIONAL = 'Professional',
}

@Entity('users')
export class User {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column({ unique: true })
  email: string;

  @Column()
  name: string;

  @Column({ type: 'varchar', nullable: true })
  googleId: string | null;

  @Column({ type: 'varchar', nullable: true, select: false })
  password: string | null;

  @Column({ type: 'boolean', default: false })
  isPasswordChanged: boolean;

  @Column({ type: 'varchar', nullable: true })
  avatarUrl: string | null;

  // Multi-role support (Rule 18): stored as JSON array e.g. ["admin","mentor"]
  @Column({ type: 'simple-json', default: '["student"]' })
  roles: UserRole[];

  @Column({ type: 'enum', enum: UserStatus, default: UserStatus.INVITED })
  status: UserStatus;

  @Column({ type: 'timestamp', nullable: true })
  lastLoginAt: Date;

  @CreateDateColumn()
  createdAt: Date;

  @Column({ type: 'varchar', nullable: true })
  whatsapp: string | null;

  @Column({ type: 'varchar', nullable: true })
  institution: string | null;

  @Column({ type: 'varchar', nullable: true })
  studyProgram: string | null;

  @Column({ type: 'varchar', nullable: true })
  selectedProgram: string | null;

  @Column({ type: 'varchar', nullable: true })
  programId: string | null;

  @Column({ type: 'varchar', nullable: true })
  specialization: string | null;

  // Explicit batch assignments array stored directly on user entity
  @Column({ type: 'simple-json', nullable: true, default: '[]' })
  assignedBatchIds: string[] | null;

  // AI & External API Credentials Configuration
  @Column({ type: 'varchar', nullable: true })
  githubToken: string | null;

  @Column({ type: 'varchar', nullable: true })
  figmaToken: string | null;

  @Column({ type: 'varchar', nullable: true })
  googleAiStudioKey: string | null;

  @Column({ type: 'varchar', nullable: true })
  groqApiKey: string | null;

  @Column({ type: 'varchar', default: 'ollama' })
  aiProvider: string;

  @Column({ type: 'varchar', default: 'http://localhost:11434' })
  ollamaHost: string;

  @Column({ type: 'varchar', nullable: true })
  selectedModel: string | null;

  @Column({ type: 'varchar', nullable: true })
  selectedOllamaModel: string | null;

  @Column({ type: 'varchar', nullable: true })
  selectedGroqModel: string | null;

  @Column({ type: 'varchar', nullable: true })
  selectedGeminiModel: string | null;

  @UpdateDateColumn()
  updatedAt: Date;

  // Helper methods for role checking
  hasRole(role: UserRole): boolean {
    return this.roles && this.roles.includes(role);
  }

  get isAdmin(): boolean {
    return this.hasRole(UserRole.ADMIN);
  }

  get isFacilitator(): boolean {
    return this.hasRole(UserRole.FACILITATOR);
  }

  get isMentor(): boolean {
    return this.hasRole(UserRole.MENTOR);
  }

  get isStudent(): boolean {
    return this.hasRole(UserRole.STUDENT);
  }

  // Backward-compat getter: returns the "primary" role for display purposes
  get role(): UserRole {
    if (this.roles?.includes(UserRole.ADMIN)) return UserRole.ADMIN;
    if (this.roles?.includes(UserRole.FACILITATOR)) return UserRole.FACILITATOR;
    if (this.roles?.includes(UserRole.MENTOR)) return UserRole.MENTOR;
    return UserRole.STUDENT;
  }

  toJSON() {
    return {
      id: this.id,
      email: this.email,
      name: this.name,
      avatarUrl: this.avatarUrl,
      roles: this.roles,
      role: this.role,
      status: this.status,
      lastLoginAt: this.lastLoginAt,
      createdAt: this.createdAt,
      whatsapp: this.whatsapp,
      institution: this.institution,
      studyProgram: this.studyProgram,
      selectedProgram: this.selectedProgram,
      programId: this.programId,
      specialization: this.specialization,
      aiProvider: this.aiProvider,
      selectedModel: this.selectedModel,
      updatedAt: this.updatedAt,
    };
  }
}
