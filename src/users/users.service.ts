import {
  Injectable,
  ConflictException,
  NotFoundException,
  BadRequestException,
  ForbiddenException,
} from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, In, DataSource, Raw } from 'typeorm';
import { User, UserStatus, UserRole } from './entities/user.entity.js';
import { CreateUserDto } from './dto/create-user.dto.js';
import { UpdateUserDto } from './dto/update-user.dto.js';
import { BulkInviteDto } from './dto/bulk-invite.dto.js';
import { MailService } from './mail.service.js';
import { StorageService } from '../storage/storage.service.js';
import * as bcrypt from 'bcryptjs';

@Injectable()
export class UsersService {
  constructor(
    @InjectRepository(User)
    private readonly usersRepository: Repository<User>,
    private readonly mailService: MailService,
    private readonly dataSource: DataSource,
    private readonly storageService: StorageService,
  ) {}

  async findByEmail(email: string): Promise<User | null> {
    const cleanEmail = (email || '').trim().toLowerCase();
    return this.usersRepository.findOne({
      where: {
        email: Raw((alias) => `LOWER(TRIM(${alias})) = :email`, {
          email: cleanEmail,
        }),
      },
    });
  }

  async findByEmailWithPassword(email: string): Promise<User | null> {
    const cleanEmail = (email || '').trim().toLowerCase();
    return this.usersRepository.findOne({
      where: {
        email: Raw((alias) => `LOWER(TRIM(${alias})) = :email`, {
          email: cleanEmail,
        }),
      },
      select: {
        id: true,
        email: true,
        name: true,
        password: true,
        googleId: true,
        roles: true,
        status: true,
        avatarUrl: true,
        whatsapp: true,
        institution: true,
        studyProgram: true,
        selectedProgram: true,
        programId: true,
        specialization: true,
        assignedBatchIds: true,
      },
    });
  }

  async findById(id: string): Promise<User | null> {
    return this.usersRepository.findOne({ where: { id } });
  }

  async saveUser(user: User): Promise<User> {
    return this.usersRepository.save(user);
  }

  private async autoRepairMissingStudentEnrollments(users: User[]) {
    try {
      // Fast check: Find students that have NO enrollments at all
      const unassignedStudents = await this.dataSource.query(`
        SELECT u.id, u.email, u."programId", u."selectedProgram"
        FROM users u
        LEFT JOIN enrollments e ON u.id = e."studentId"
        WHERE e.id IS NULL AND (u.roles::text LIKE '%student%')
      `);

      if (!unassignedStudents || unassignedStudents.length === 0) return;

      const activeBatchRes = await this.dataSource.query(
        `SELECT id FROM batches WHERE LOWER(status::text) = 'active' LIMIT 1`,
      );
      if (activeBatchRes.length === 0) return;
      const activeBatchId = activeBatchRes[0].id;

      const allPrograms = await this.dataSource.query(
        `SELECT id, name FROM programs`,
      );
      if (allPrograms.length === 0) return;

      for (const student of unassignedStudents) {
        let progId = student.programId;
        if (!progId && student.selectedProgram) {
          const matchedProg = allPrograms.find(
            (p: any) =>
              p.name.trim().toLowerCase() ===
              student.selectedProgram?.trim().toLowerCase(),
          );
          if (matchedProg) progId = matchedProg.id;
        }
        if (!progId) progId = allPrograms[0].id;

        const classRes = await this.dataSource.query(
          `SELECT id FROM classes WHERE "programId" = $1 AND "batchId" = $2 LIMIT 1`,
          [progId, activeBatchId],
        );
        let classId: string;
        if (classRes.length > 0) {
          classId = classRes[0].id;
        } else {
          const newClass = await this.dataSource.query(
            `INSERT INTO classes (id, "programId", "batchId", "createdAt", "updatedAt") VALUES (gen_random_uuid(), $1, $2, NOW(), NOW()) RETURNING id`,
            [progId, activeBatchId],
          );
          classId = newClass[0].id;
        }

        await this.dataSource.query(
          `INSERT INTO enrollments (id, "studentId", "classId", "createdAt") VALUES (gen_random_uuid(), $1, $2, NOW()) ON CONFLICT DO NOTHING`,
          [student.id, classId],
        );
        console.log(
          `[AutoRepair] Successfully auto-enrolled student ${student.email} into active batch ${activeBatchId}`,
        );
      }
    } catch (err) {
      console.error('[AutoRepair Error]', err);
    }
  }

  async findAll(): Promise<any[]> {
    try {
      console.log(
        '[DB DIAGNOSTIC] Fetching all users from Supabase PostgreSQL...',
      );
      const users = await this.usersRepository.find({
        order: { createdAt: 'DESC' },
      });
      console.log(
        `[DB DIAGNOSTIC] Found ${users.length} raw user entities in DB.`,
      );

      await this.autoRepairMissingStudentEnrollments(users);

      // Fetch student enrollments
      const studentEnrollments = await this.dataSource.query(`
        SELECT e."studentId", b.id as "batchId", b.name as "batchName"
        FROM enrollments e
        JOIN classes c ON e."classId" = c.id
        JOIN batches b ON c."batchId" = b.id
      `);

      // Fetch mentor class assignments
      const mentorClasses = await this.dataSource.query(`
        SELECT c."mentorId", b.id as "batchId", b.name as "batchName"
        FROM classes c
        JOIN batches b ON c."batchId" = b.id
        WHERE c."mentorId" IS NOT NULL
      `);

      // Fetch all batches for direct ID lookup
      const allBatches = await this.dataSource.query(
        `SELECT id, name FROM batches`,
      );

      const result = users.map((user) => {
        const userBatches: any[] = [];
        const roles = (user.roles || []).map((r) => String(r).toLowerCase());
        const userRoleStr = String(user.role || '').toLowerCase();
        const isStudent =
          roles.includes('student') || userRoleStr === 'student';
        const isMentor = roles.includes('mentor') || userRoleStr === 'mentor';

        if (isStudent) {
          const enrolls = studentEnrollments.filter(
            (e: any) => e.studentId === user.id,
          );
          enrolls.forEach((e: any) =>
            userBatches.push({ id: e.batchId, name: e.batchName }),
          );
        }
        if (isMentor) {
          const classes = mentorClasses.filter(
            (c: any) => c.mentorId === user.id,
          );
          classes.forEach((c: any) =>
            userBatches.push({ id: c.batchId, name: c.batchName }),
          );
        }

        // Merge explicit assignedBatchIds stored directly on user entity
        if (
          Array.isArray(user.assignedBatchIds) &&
          user.assignedBatchIds.length > 0
        ) {
          user.assignedBatchIds.forEach((bId) => {
            const matched = allBatches.find((b: any) => b.id === bId);
            if (matched) {
              userBatches.push({ id: matched.id, name: matched.name });
            }
          });
        }

        // Deduplicate batches
        const uniqueBatches = Array.from(
          new Map(userBatches.map((b) => [b.id, b])).values(),
        );

        return {
          ...user.toJSON(),
          batches: uniqueBatches,
        };
      });

      console.log(
        `[DB DIAGNOSTIC SUCCESS] Formatted ${result.length} user records.`,
      );
      return result;
    } catch (dbError: any) {
      console.error(
        '[DB DIAGNOSTIC ERROR] Failed to fetch users from database:',
        dbError,
      );
      throw dbError;
    }
  }

  async invite(dto: CreateUserDto): Promise<User> {
    const email = dto.email.toLowerCase().trim();
    const existing = await this.findByEmail(email);

    if (existing) {
      throw new ConflictException(
        `User dengan email ${email} sudah terdaftar.`,
      );
    }

    const rawRoles = dto.roles || (dto.role ? [dto.role] : [UserRole.STUDENT]);
    const targetRoles = rawRoles.map(
      (r) => String(r).toLowerCase() as UserRole,
    );

    if (targetRoles.includes(UserRole.FACILITATOR)) {
      if (!dto.selectedProgram && !dto.programId) {
        throw new BadRequestException(
          'Facilitator wajib dipautkan dengan Program tertentu.',
        );
      }
    }
    if (
      targetRoles.includes(UserRole.STUDENT) ||
      targetRoles.includes(UserRole.MENTOR)
    ) {
      if (!dto.selectedProgram) {
        throw new BadRequestException(
          'Setiap Student dan Mentor wajib memiliki program studi (Rule 29).',
        );
      }
      const activeBatches = await this.dataSource.query(
        `SELECT id FROM batches WHERE LOWER(status::text) = 'active'`,
      );
      if (activeBatches.length === 0) {
        throw new BadRequestException(
          'Pendaftaran dibatalkan: Tidak ada Batch/Cohort aktif. Silakan buat atau aktifkan Batch terlebih dahulu.',
        );
      }
    }

    let programId = dto.programId || null;
    let selectedProgram = dto.selectedProgram || null;

    if (selectedProgram && !programId) {
      const progRes = await this.dataSource.query(
        `SELECT id FROM programs WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) LIMIT 1`,
        [selectedProgram],
      );
      if (progRes.length > 0) {
        programId = progRes[0].id;
      }
    } else if (programId && !selectedProgram) {
      const progRes = await this.dataSource.query(
        `SELECT name FROM programs WHERE id = $1 LIMIT 1`,
        [programId],
      );
      if (progRes.length > 0) {
        selectedProgram = progRes[0].name;
      }
    }

    const isGmail = email.endsWith('@gmail.com');
    const defaultPassword = isGmail
      ? null
      : await bcrypt.hash('Student123!', 10);
    const isPasswordChanged = isGmail ? true : false;

    const user = this.usersRepository.create({
      name: dto.name,
      email,
      password: defaultPassword,
      isPasswordChanged,
      roles: targetRoles,
      status: UserStatus.INVITED,
      whatsapp: dto.whatsapp || null,
      institution: dto.institution || null,
      studyProgram: dto.studyProgram || null,
      selectedProgram: selectedProgram,
      programId: programId,
      specialization: dto.specialization || null,
    });

    const savedUser = await this.usersRepository.save(user);

    // Auto-enroll student into active batch class for their selected program
    if (
      targetRoles.includes(UserRole.STUDENT) ||
      targetRoles.includes(UserRole.MENTOR)
    ) {
      try {
        const activeBatchRes = await this.dataSource.query(
          `SELECT id FROM batches WHERE LOWER(status::text) = 'active' LIMIT 1`,
        );
        if (activeBatchRes.length > 0) {
          const activeBatchId = activeBatchRes[0].id;
          savedUser.assignedBatchIds = [activeBatchId];
          await this.usersRepository.save(savedUser);
          console.log(
            `[INVITE DIAGNOSTIC] Saved assignedBatchIds for invited user ${savedUser.email}:`,
            savedUser.assignedBatchIds,
          );

          const progRes = await this.dataSource.query(
            `SELECT id FROM programs WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) LIMIT 1`,
            [dto.selectedProgram || ''],
          );
          const resolvedProgId = progRes.length > 0 ? progRes[0].id : programId;

          if (resolvedProgId) {
            const classRes = await this.dataSource.query(
              `SELECT id FROM classes WHERE "programId" = $1 AND "batchId" = $2 LIMIT 1`,
              [resolvedProgId, activeBatchId],
            );
            let classId: string;
            if (classRes.length > 0) {
              classId = classRes[0].id;
            } else {
              const newClass = await this.dataSource.query(
                `INSERT INTO classes (id, "programId", "batchId", "createdAt", "updatedAt") VALUES (gen_random_uuid(), $1, $2, NOW(), NOW()) RETURNING id`,
                [resolvedProgId, activeBatchId],
              );
              classId = newClass[0].id;
            }

            if (targetRoles.includes(UserRole.STUDENT)) {
              await this.dataSource.query(
                `INSERT INTO enrollments (id, "studentId", "classId", "createdAt") VALUES (gen_random_uuid(), $1, $2, NOW()) ON CONFLICT DO NOTHING`,
                [savedUser.id, classId],
              );
              console.log(
                `[INVITE DIAGNOSTIC] Auto-enrolled student ${savedUser.email} into class ${classId}`,
              );
            } else if (targetRoles.includes(UserRole.MENTOR)) {
              await this.dataSource.query(
                `UPDATE classes SET "mentorId" = $1 WHERE id = $2`,
                [savedUser.id, classId],
              );
              console.log(
                `[INVITE DIAGNOSTIC] Assigned mentor ${savedUser.email} to class ${classId}`,
              );
            }
          }
        }
      } catch (autoEnrollErr) {
        console.error(
          `[INVITE DIAGNOSTIC ERROR] Auto-enrollment error for ${savedUser.email}:`,
          autoEnrollErr,
        );
      }
    }

    return savedUser;
  }

  async bulkInvite(dto: BulkInviteDto) {
    const results = {
      invited: [] as { email: string; name: string; role: string }[],
      failed: [] as { email: string; reason: string }[],
    };

    // Option 1: Structured JSON array
    if (dto.users && dto.users.length > 0) {
      for (const userDto of dto.users) {
        try {
          if (userDto.selectedProgram) {
            const lowerProg = userDto.selectedProgram.toLowerCase();
            if (lowerProg.includes('ai') || lowerProg.includes('artificial')) {
              userDto.selectedProgram = 'AI Development';
            } else if (
              lowerProg.includes('web') ||
              lowerProg.includes('front')
            ) {
              userDto.selectedProgram = 'Web Development and UI/UX Design';
            } else if (
              lowerProg.includes('mobile') ||
              lowerProg.includes('android') ||
              lowerProg.includes('ios')
            ) {
              userDto.selectedProgram = 'Mobile Development and UI/UX Design';
            } else if (
              lowerProg.includes('game') ||
              lowerProg.includes('unity')
            ) {
              userDto.selectedProgram = 'Game Development';
            }
          }
          const invitedUser = await this.invite(userDto);
          results.invited.push({
            email: invitedUser.email,
            name: invitedUser.name,
            role: invitedUser.role,
          });
        } catch (err: any) {
          results.failed.push({
            email: userDto.email,
            reason: err.message || 'Unknown error',
          });
        }
      }
    }

    // Option 2: Copy-pasted CSV raw string
    if (dto.rawEmails) {
      const lines = dto.rawEmails
        .split(/\r?\n/)
        .map((l) => l.trim())
        .filter((l) => l.length > 0);

      if (lines.length === 0) {
        return results;
      }

      // Helper to capitalize words (normalization)
      const normalizeName = (str: string): string => {
        if (!str) return '';
        return str
          .toLowerCase()
          .split(/\s+/)
          .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
          .join(' ')
          .trim();
      };

      // Helper to parse CSV fields, handling values in quotes
      const parseCSVLine = (line: string, delimiter: string): string[] => {
        const result: string[] = [];
        let current = '';
        let inQuotes = false;
        for (let i = 0; i < line.length; i++) {
          const char = line[i];
          if (char === '"') {
            inQuotes = !inQuotes;
          } else if (char === delimiter && !inQuotes) {
            result.push(current.trim());
            current = '';
          } else {
            current += char;
          }
        }
        result.push(current.trim());
        return result.map((f) => f.replace(/^"|"$/g, '').trim());
      };

      // Auto-detect CSV delimiter (comma or tab)
      const firstLine = lines[0];
      const tabCount = (firstLine.match(/\t/g) || []).length;
      const commaCount = (firstLine.match(/,/g) || []).length;
      const delimiter = tabCount > commaCount ? '\t' : ',';

      const firstFields = parseCSVLine(firstLine, delimiter);
      // Heuristic to check if the first row is a header
      const hasHeader = firstFields.some((field) =>
        /email|nama|peserta|institusi|whatsapp|program/i.test(field),
      );

      let headers: string[] = [];
      let dataLines = lines;

      if (hasHeader) {
        headers = firstFields.map((h) => h.toLowerCase().trim());
        dataLines = lines.slice(1);
      }

      // Dynamic header mapping matching
      const findHeaderIndex = (
        patterns: RegExp[],
        excludePatterns: RegExp[] = [],
      ): number => {
        return headers.findIndex(
          (h) =>
            patterns.some((p) => p.test(h)) &&
            !excludePatterns.some((ep) => ep.test(h)),
        );
      };

      const nameIdx = findHeaderIndex(
        [/nama.*lengkap|lengkap.*nama|nama.*peserta|^nama$/i],
        [/kaprodi|dosen/i],
      );
      const emailIdx = findHeaderIndex([/email|^surel$/i], [/kaprodi|dosen/i]);
      const waIdx = findHeaderIndex(
        [/whatsapp|no.*wa|no.*hp|telepon/i],
        [/kaprodi|dosen/i],
      );
      const instIdx = findHeaderIndex([
        /institusi|kampus|universitas|sekolah/i,
      ]);
      const progIdx = findHeaderIndex([
        /program.*il|pilihan.*program|program.*dipilih/i,
      ]);
      const studyIdx = findHeaderIndex([/program.*studi|jurusan|prodi/i]);

      const isPlainEmailList = !hasHeader || emailIdx === -1;

      if (isPlainEmailList) {
        // Fallback: Parse as plain emails list (newline or comma separated)
        const emailList = dto.rawEmails
          .split(/[\n,;]+/)
          .map((e) => e.trim())
          .filter((e) => e.length > 0);

        for (const rawEmail of emailList) {
          try {
            if (!rawEmail.includes('@')) {
              throw new BadRequestException('Format email tidak valid.');
            }
            const namePrefix = rawEmail.split('@')[0];
            const userDto: CreateUserDto = {
              email: rawEmail,
              name: normalizeName(namePrefix.replace(/[._-]/g, ' ')),
              role: dto.defaultRole || UserRole.STUDENT,
            };
            const invitedUser = await this.invite(userDto);
            results.invited.push({
              email: invitedUser.email,
              name: invitedUser.name,
              role: invitedUser.role,
            });
          } catch (err: any) {
            results.failed.push({
              email: rawEmail,
              reason: err.message || 'Unknown error',
            });
          }
        }
      } else {
        // Option 2: Parse actual CSV records
        for (const line of dataLines) {
          const fields = parseCSVLine(line, delimiter);
          const email =
            emailIdx !== -1 && fields[emailIdx]
              ? fields[emailIdx].toLowerCase().trim()
              : '';

          try {
            if (!email || !email.includes('@')) {
              throw new Error('Alamat email kosong atau tidak valid.');
            }

            // Resolve name and normalize
            let rawName =
              nameIdx !== -1 && fields[nameIdx] ? fields[nameIdx] : '';
            if (!rawName) {
              rawName = email.split('@')[0].replace(/[._-]/g, ' ');
            }
            const name = normalizeName(rawName);

            // Resolve institution and normalize
            const rawInst =
              instIdx !== -1 && fields[instIdx] ? fields[instIdx] : '';
            const institution = normalizeName(rawInst) || null;

            // Clean whatsapp formatting
            const rawWa = waIdx !== -1 && fields[waIdx] ? fields[waIdx] : '';
            const whatsapp = rawWa.replace(/[^0-9+]/g, '') || null;

            // Resolve study program
            const studyProgram =
              studyIdx !== -1 && fields[studyIdx] ? fields[studyIdx] : null;

            // Resolve and map selected Program IL to standard list
            const rawProg =
              progIdx !== -1 && fields[progIdx] ? fields[progIdx] : '';
            let selectedProgram = rawProg || null;
            if (rawProg) {
              const lowerProg = rawProg.toLowerCase();
              if (
                lowerProg.includes('ai') ||
                lowerProg.includes('artificial')
              ) {
                selectedProgram = 'AI Development';
              } else if (
                lowerProg.includes('web') ||
                lowerProg.includes('front')
              ) {
                selectedProgram = 'Web Development and UI/UX Design';
              } else if (
                lowerProg.includes('mobile') ||
                lowerProg.includes('android') ||
                lowerProg.includes('ios')
              ) {
                selectedProgram = 'Mobile Development and UI/UX Design';
              } else if (
                lowerProg.includes('game') ||
                lowerProg.includes('unity')
              ) {
                selectedProgram = 'Game Development';
              }
            }

            const userDto: CreateUserDto = {
              email,
              name,
              role: dto.defaultRole || UserRole.STUDENT,
              whatsapp: whatsapp ?? undefined,
              institution: institution ?? undefined,
              studyProgram: studyProgram ?? undefined,
              selectedProgram: selectedProgram ?? undefined,
            };

            const invitedUser = await this.invite(userDto);
            results.invited.push({
              email: invitedUser.email,
              name: invitedUser.name,
              role: invitedUser.role,
            });
          } catch (err: any) {
            results.failed.push({
              email: email || 'Baris data',
              reason: err.message || 'Error parsing',
            });
          }
        }
      }
    }

    return results;
  }

  async activateOnFirstLogin(
    user: User,
    googleId: string,
    avatarUrl: string | null,
  ): Promise<User> {
    let r2AvatarUrl = user.avatarUrl;
    if (avatarUrl) {
      r2AvatarUrl = await this.storageService.uploadUrlToR2(
        avatarUrl,
        'avatars',
      );
    }

    await this.usersRepository.update(user.id, {
      googleId,
      avatarUrl: r2AvatarUrl ?? undefined,
      status: UserStatus.ACTIVE,
      lastLoginAt: new Date(),
    });

    return (await this.findById(user.id))!;
  }

  async updateLastLogin(user: User): Promise<void> {
    // If user avatar URL is external (e.g. Google's domain) and not on R2 yet, upload to R2
    if (user.avatarUrl && !user.avatarUrl.includes('lms-v2')) {
      try {
        const r2Url = await this.storageService.uploadUrlToR2(
          user.avatarUrl,
          'avatars',
        );
        if (r2Url && r2Url !== user.avatarUrl) {
          await this.usersRepository.update(user.id, {
            avatarUrl: r2Url,
            lastLoginAt: new Date(),
          });
          return;
        }
      } catch (e) {}
    }

    await this.usersRepository.update(user.id, {
      lastLoginAt: new Date(),
    });
  }

  private async ensureNotLastAdmin(
    userId: string,
    action: string,
  ): Promise<void> {
    const user = await this.findById(userId);
    if (!user || !user.roles.includes(UserRole.ADMIN)) return;

    const activeAdmins = await this.usersRepository.count({
      where: {
        roles:
          process.env.NODE_ENV === 'test'
            ? undefined
            : this.dataSource.driver.options.type === 'postgres'
              ? (Raw((alias) => `${alias}::jsonb @> '["admin"]'`) as any)
              : undefined,
        status: UserStatus.ACTIVE,
      },
    });

    // Fallback manual count if raw query count isn't reliable for simple-json
    let actualAdminCount = activeAdmins;
    if (this.dataSource.driver.options.type !== 'postgres') {
      const allUsers = await this.usersRepository.find({
        where: { status: UserStatus.ACTIVE },
      });
      actualAdminCount = allUsers.filter(
        (u) => u.roles && u.roles.includes(UserRole.ADMIN),
      ).length;
    }

    if (actualAdminCount <= 1) {
      throw new ForbiddenException(
        `Tidak dapat ${action} admin terakhir yang aktif dalam sistem.`,
      );
    }
  }

  async suspend(id: string, caller?: User): Promise<User> {
    if (caller && caller.id === id) {
      throw new ForbiddenException(
        'Anda tidak dapat mengubah status Anda sendiri.',
      );
    }
    await this.ensureNotLastAdmin(id, 'menskors (suspend)');
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    user.status = UserStatus.SUSPENDED;
    return this.usersRepository.save(user);
  }

  async unsuspend(id: string, caller?: User): Promise<User> {
    if (caller && caller.id === id) {
      throw new ForbiddenException(
        'Anda tidak dapat mengubah status Anda sendiri.',
      );
    }
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    // Set status based on Google ID presence
    user.status = user.googleId ? UserStatus.ACTIVE : UserStatus.INVITED;
    return this.usersRepository.save(user);
  }

  // Mitigation tool: Admin can correct the email address of any user (e.g. typos).
  // Resets Google ID binding to allow re-login.
  async updateEmail(
    id: string,
    newEmail: string,
    caller?: User,
  ): Promise<User> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    const cleanedEmail = newEmail.toLowerCase().trim();
    if (user.email === cleanedEmail) {
      return user;
    }

    const existing = await this.findByEmail(cleanedEmail);
    if (existing && existing.id !== id) {
      throw new ConflictException(
        `Email ${cleanedEmail} sudah terdaftar di sistem.`,
      );
    }

    user.email = cleanedEmail;
    user.googleId = null; // Reset Google Auth binding
    user.status = UserStatus.INVITED; // Require re-verification/login

    const updatedUser = await this.usersRepository.save(user);

    return updatedUser;
  }

  async update(id: string, dto: UpdateUserDto, caller?: User): Promise<User> {
    if (caller && caller.id === id) {
      if (dto.roles) {
        throw new ForbiddenException(
          'Anda tidak dapat mengubah role Anda sendiri.',
        );
      }
      if (dto.status) {
        throw new ForbiddenException(
          'Anda tidak dapat mengubah status Anda sendiri.',
        );
      }
    }

    if (dto.roles && !dto.roles.includes(UserRole.ADMIN)) {
      await this.ensureNotLastAdmin(id, 'menghapus role admin dari');
    }

    if (dto.status && dto.status !== UserStatus.ACTIVE) {
      await this.ensureNotLastAdmin(id, 'menonaktifkan');
    }

    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    const targetBatchIds = dto.batchIds || dto.assignedBatchIds;
    console.log(
      `[UPDATE USER DIAGNOSTIC] Updating User ID: ${id}, Name: ${user.name}, Email: ${user.email}`,
    );
    console.log(
      `[UPDATE USER DIAGNOSTIC] Incoming batchIds payload:`,
      targetBatchIds,
    );

    if (targetBatchIds && Array.isArray(targetBatchIds)) {
      user.assignedBatchIds = targetBatchIds;
    }

    if (dto.avatarUrl) {
      if (dto.avatarUrl.startsWith('data:image')) {
        dto.avatarUrl = await this.storageService.uploadBase64(
          dto.avatarUrl,
          'avatars',
        );
      } else if (
        dto.avatarUrl.startsWith('http://') ||
        dto.avatarUrl.startsWith('https://')
      ) {
        dto.avatarUrl = await this.storageService.uploadUrlToR2(
          dto.avatarUrl,
          'avatars',
        );
      }
    }

    const { batchIds, assignedBatchIds, ...userFields } = dto;
    Object.assign(user, userFields);

    // Normalize roles to lowercased enum values to clean up legacy DB entries (e.g. ['STUDENT'] -> ['student'])
    if (user.roles && Array.isArray(user.roles)) {
      user.roles = user.roles.map((r) => String(r).toLowerCase() as UserRole);
    }
    const savedUser = await this.usersRepository.save(user);
    console.log(
      `[UPDATE USER DIAGNOSTIC SUCCESS] Saved User entity. assignedBatchIds:`,
      savedUser.assignedBatchIds,
    );

    if (batchIds && Array.isArray(batchIds)) {
      const roles = (user.roles || []).map((r) => String(r).toLowerCase());
      const userRoleStr = String((user as any).role || '').toLowerCase();
      const isStudent = roles.includes('student') || userRoleStr === 'student';
      const isMentor = roles.includes('mentor') || userRoleStr === 'mentor';

      if (isStudent) {
        // Fetch current enrollments for this student with class & batch
        const currentEnrolls = await this.dataSource.query(
          `SELECT e.id as "enrollmentId", c."batchId", c.id as "classId"
           FROM enrollments e
           JOIN classes c ON e."classId" = c.id
           WHERE e."studentId" = $1`,
          [id],
        );

        const currentBatchIds = currentEnrolls.map((e: any) => e.batchId);

        // 1. Remove enrollments for batches NOT in batchIds
        for (const curr of currentEnrolls) {
          if (!batchIds.includes(curr.batchId)) {
            await this.dataSource.query(
              `DELETE FROM enrollments WHERE id = $1`,
              [curr.enrollmentId],
            );
          }
        }

        // 2. Add enrollments for newly checked batchIds
        for (const targetBatchId of batchIds) {
          if (!currentBatchIds.includes(targetBatchId)) {
            let progId = user.programId;
            if (!progId && user.selectedProgram) {
              const pRes = await this.dataSource.query(
                `SELECT id FROM programs WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) LIMIT 1`,
                [user.selectedProgram],
              );
              if (pRes.length > 0) progId = pRes[0].id;
            }

            if (!progId) {
              const defaultProg = await this.dataSource.query(
                `SELECT id FROM programs LIMIT 1`,
              );
              if (defaultProg.length > 0) progId = defaultProg[0].id;
            }

            if (progId) {
              const classRes = await this.dataSource.query(
                `SELECT id FROM classes WHERE "programId" = $1 AND "batchId" = $2 LIMIT 1`,
                [progId, targetBatchId],
              );
              let targetClassId: string;
              if (classRes.length > 0) {
                targetClassId = classRes[0].id;
              } else {
                const newClass = await this.dataSource.query(
                  `INSERT INTO classes (id, "programId", "batchId", "createdAt", "updatedAt") VALUES (gen_random_uuid(), $1, $2, NOW(), NOW()) RETURNING id`,
                  [progId, targetBatchId],
                );
                targetClassId = newClass[0].id;
              }

              await this.dataSource.query(
                `INSERT INTO enrollments (id, "studentId", "classId", "createdAt") VALUES (gen_random_uuid(), $1, $2, NOW()) ON CONFLICT DO NOTHING`,
                [id, targetClassId],
              );
            }
          }
        }
      }

      if (isMentor) {
        // 1. Unassign mentor from classes in batches NOT in batchIds
        const currentMentorClasses = await this.dataSource.query(
          `SELECT id, "batchId" FROM classes WHERE "mentorId" = $1`,
          [id],
        );
        for (const cls of currentMentorClasses) {
          if (!batchIds.includes(cls.batchId)) {
            await this.dataSource.query(
              `UPDATE classes SET "mentorId" = NULL WHERE id = $1`,
              [cls.id],
            );
          }
        }

        // 2. Assign mentor to classes in target batchIds
        for (const targetBatchId of batchIds) {
          let progId = user.programId;
          if (!progId && user.selectedProgram) {
            const pRes = await this.dataSource.query(
              `SELECT id FROM programs WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) LIMIT 1`,
              [user.selectedProgram],
            );
            if (pRes.length > 0) progId = pRes[0].id;
          }
          if (!progId) {
            const defaultProg = await this.dataSource.query(
              `SELECT id FROM programs LIMIT 1`,
            );
            if (defaultProg.length > 0) progId = defaultProg[0].id;
          }
          if (progId) {
            const classRes = await this.dataSource.query(
              `SELECT id FROM classes WHERE "programId" = $1 AND "batchId" = $2 LIMIT 1`,
              [progId, targetBatchId],
            );
            if (classRes.length > 0) {
              await this.dataSource.query(
                `UPDATE classes SET "mentorId" = $1 WHERE id = $2`,
                [id, classRes[0].id],
              );
            } else {
              await this.dataSource.query(
                `INSERT INTO classes (id, "programId", "batchId", "mentorId", "createdAt", "updatedAt") VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
                [progId, targetBatchId, id],
              );
            }
          }
        }
      }
    }

    return savedUser;
  }

  async remove(id: string, caller?: User): Promise<void> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    await this.ensureNotLastAdmin(id, 'menghapus');

    if (user.roles.includes(UserRole.STUDENT)) {
      const activeEnrollments = await this.dataSource.query(
        `
        SELECT COUNT(*) as count FROM enrollments e
        INNER JOIN classes c ON e."classId" = c.id
        INNER JOIN batches b ON c."batchId" = b.id
        WHERE e."studentId" = $1 AND b.status = 'active'
      `,
        [id],
      );
      if (parseInt(activeEnrollments[0].count, 10) > 0) {
        throw new ForbiddenException(
          'Siswa tidak dapat dihapus karena terdaftar pada Batch yang sedang aktif (Rule 19).',
        );
      }
    }

    if (user.roles.includes(UserRole.MENTOR)) {
      const activeClasses = await this.dataSource.query(
        `
        SELECT COUNT(*) as count FROM classes c
        INNER JOIN batches b ON c."batchId" = b.id
        WHERE c."mentorId" = $1 AND b.status = 'active'
      `,
        [id],
      );
      if (parseInt(activeClasses[0].count, 10) > 0) {
        throw new ForbiddenException(
          'Mentor tidak dapat dihapus karena bertugas pada Batch yang sedang aktif (Rule 19).',
        );
      }
    }

    // Clean up relational foreign keys before deleting user to prevent FK constraint error 23503
    await this.dataSource.query(
      'UPDATE classes SET "mentorId" = NULL WHERE "mentorId" = $1',
      [id],
    );
    await this.dataSource.query(
      'UPDATE competencies SET "creatorMentorId" = NULL WHERE "creatorMentorId" = $1',
      [id],
    );
    await this.dataSource.query(
      'UPDATE rubrik_assessments SET "creatorMentorId" = NULL WHERE "creatorMentorId" = $1',
      [id],
    );
    await this.dataSource.query(
      'DELETE FROM enrollments WHERE "studentId" = $1',
      [id],
    );
    await this.dataSource.query(
      'DELETE FROM attendances WHERE "studentId" = $1',
      [id],
    );
    await this.dataSource.query(
      'DELETE FROM permission_requests WHERE "studentId" = $1',
      [id],
    );
    await this.dataSource.query('DELETE FROM logbooks WHERE "studentId" = $1', [
      id,
    ]);
    await this.dataSource.query(
      'DELETE FROM submissions WHERE "studentId" = $1',
      [id],
    );
    await this.dataSource.query(
      'DELETE FROM rubrik_assessment_scores WHERE "studentId" = $1',
      [id],
    );

    await this.usersRepository.remove(user);
  }

  async bulkDelete(ids: string[]): Promise<{ deletedCount: number }> {
    if (!ids || ids.length === 0) {
      throw new BadRequestException('ID pengguna tidak boleh kosong.');
    }

    const users = await this.usersRepository.find({
      where: { id: In(ids) },
    });

    const nonAdminIds = users
      .filter((u) => !u.roles.includes(UserRole.ADMIN))
      .map((u) => u.id);

    if (nonAdminIds.length === 0) {
      return { deletedCount: 0 };
    }

    for (const u of users) {
      if (!nonAdminIds.includes(u.id)) continue;

      if (u.roles.includes(UserRole.STUDENT)) {
        const activeEnrollments = await this.dataSource.query(
          `
          SELECT COUNT(*) as count FROM enrollments e
          INNER JOIN classes c ON e."classId" = c.id
          INNER JOIN batches b ON c."batchId" = b.id
          WHERE e."studentId" = $1 AND b.status = 'active'
        `,
          [u.id],
        );
        if (parseInt(activeEnrollments[0].count, 10) > 0) {
          throw new ForbiddenException(
            `Siswa ${u.name} tidak dapat dihapus karena terdaftar pada Batch yang sedang aktif (Rule 19).`,
          );
        }
      }

      if (u.roles.includes(UserRole.MENTOR)) {
        const activeClasses = await this.dataSource.query(
          `
          SELECT COUNT(*) as count FROM classes c
          INNER JOIN batches b ON c."batchId" = b.id
          WHERE c."mentorId" = $1 AND b.status = 'active'
        `,
          [u.id],
        );
        if (parseInt(activeClasses[0].count, 10) > 0) {
          throw new ForbiddenException(
            `Mentor ${u.name} tidak dapat dihapus karena bertugas pada Batch yang sedang aktif (Rule 19).`,
          );
        }
      }
    }

    if (nonAdminIds.length > 0) {
      await this.dataSource.query(
        `UPDATE classes SET "mentorId" = NULL WHERE "mentorId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `UPDATE competencies SET "creatorMentorId" = NULL WHERE "creatorMentorId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `UPDATE rubrik_assessments SET "creatorMentorId" = NULL WHERE "creatorMentorId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM enrollments WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM attendances WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM permission_requests WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM logbooks WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM submissions WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
      await this.dataSource.query(
        `DELETE FROM rubrik_assessment_scores WHERE "studentId" = ANY($1)`,
        [nonAdminIds],
      );
    }

    const result = await this.usersRepository.delete({
      id: In(nonAdminIds),
    });

    return { deletedCount: result.affected || 0 };
  }

  async sendWarningEmail(id: string): Promise<{ success: boolean }> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }
    if (user.email.endsWith('@gmail.com')) {
      throw new BadRequestException(
        'Pengguna menggunakan email @gmail.com, tidak memerlukan peringatan.',
      );
    }

    await this.mailService.sendWarningEmail(user.email, user.name, user.role);
    return { success: true };
  }
}
