import {
  Injectable,
  ConflictException,
  NotFoundException,
  BadRequestException,
} from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, In, DataSource } from 'typeorm';
import { User, UserStatus, UserRole } from './entities/user.entity.js';
import { CreateUserDto } from './dto/create-user.dto.js';
import { UpdateUserDto } from './dto/update-user.dto.js';
import { BulkInviteDto } from './dto/bulk-invite.dto.js';
import { MailService } from './mail.service.js';

@Injectable()
export class UsersService {
  constructor(
    @InjectRepository(User)
    private readonly usersRepository: Repository<User>,
    private readonly mailService: MailService,
    private readonly dataSource: DataSource,
  ) {}

  async findByEmail(email: string): Promise<User | null> {
    return this.usersRepository.findOne({ where: { email } });
  }

  async findById(id: string): Promise<User | null> {
    return this.usersRepository.findOne({ where: { id } });
  }

  async findAll(): Promise<User[]> {
    return this.usersRepository.find({ order: { createdAt: 'DESC' } });
  }

  async invite(dto: CreateUserDto): Promise<User> {
    const email = dto.email.toLowerCase().trim();
    const existing = await this.findByEmail(email);

    if (existing) {
      throw new ConflictException(`User dengan email ${email} sudah terdaftar.`);
    }

    const user = this.usersRepository.create({
      name: dto.name,
      email,
      role: dto.role,
      status: UserStatus.INVITED,
      whatsapp: dto.whatsapp || null,
      institution: dto.institution || null,
      studyProgram: dto.studyProgram || null,
      selectedProgram: dto.selectedProgram || null,
    });

    const savedUser = await this.usersRepository.save(user);

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
        return result.map(f => f.replace(/^"|"$/g, '').trim());
      };

      // Auto-detect CSV delimiter (comma or tab)
      const firstLine = lines[0];
      const tabCount = (firstLine.match(/\t/g) || []).length;
      const commaCount = (firstLine.match(/,/g) || []).length;
      const delimiter = tabCount > commaCount ? '\t' : ',';

      const firstFields = parseCSVLine(firstLine, delimiter);
      // Heuristic to check if the first row is a header
      const hasHeader = firstFields.some(field =>
        /email|nama|peserta|institusi|whatsapp|program/i.test(field)
      );

      let headers: string[] = [];
      let dataLines = lines;

      if (hasHeader) {
        headers = firstFields.map(h => h.toLowerCase().trim());
        dataLines = lines.slice(1);
      }

      // Dynamic header mapping matching
      const findHeaderIndex = (patterns: RegExp[], excludePatterns: RegExp[] = []): number => {
        return headers.findIndex(h =>
          patterns.some(p => p.test(h)) && !excludePatterns.some(ep => ep.test(h))
        );
      };

      const nameIdx = findHeaderIndex([/nama.*lengkap|lengkap.*nama|nama.*peserta|^nama$/i], [/kaprodi|dosen/i]);
      const emailIdx = findHeaderIndex([/email|^surel$/i], [/kaprodi|dosen/i]);
      const waIdx = findHeaderIndex([/whatsapp|no.*wa|no.*hp|telepon/i], [/kaprodi|dosen/i]);
      const instIdx = findHeaderIndex([/institusi|kampus|universitas|sekolah/i]);
      const progIdx = findHeaderIndex([/program.*il|pilihan.*program|program.*dipilih/i]);
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
          const email = emailIdx !== -1 && fields[emailIdx] ? fields[emailIdx].toLowerCase().trim() : '';

          try {
            if (!email || !email.includes('@')) {
              throw new Error('Alamat email kosong atau tidak valid.');
            }

            // Resolve name and normalize
            let rawName = nameIdx !== -1 && fields[nameIdx] ? fields[nameIdx] : '';
            if (!rawName) {
              rawName = email.split('@')[0].replace(/[._-]/g, ' ');
            }
            const name = normalizeName(rawName);

            // Resolve institution and normalize
            const rawInst = instIdx !== -1 && fields[instIdx] ? fields[instIdx] : '';
            const institution = normalizeName(rawInst) || null;

            // Clean whatsapp formatting
            const rawWa = waIdx !== -1 && fields[waIdx] ? fields[waIdx] : '';
            const whatsapp = rawWa.replace(/[^0-9+]/g, '') || null;

            // Resolve study program
            const studyProgram = studyIdx !== -1 && fields[studyIdx] ? fields[studyIdx] : null;

            // Resolve and map selected Program IL to standard list
            const rawProg = progIdx !== -1 && fields[progIdx] ? fields[progIdx] : '';
            let selectedProgram = rawProg || null;
            if (rawProg) {
              const lowerProg = rawProg.toLowerCase();
              if (lowerProg.includes('ai') || lowerProg.includes('artificial')) {
                selectedProgram = 'AI Development';
               } else if (lowerProg.includes('web') || lowerProg.includes('front')) {
                selectedProgram = 'Web Development and UI/UX Design';
              } else if (lowerProg.includes('mobile') || lowerProg.includes('android') || lowerProg.includes('ios')) {
                selectedProgram = 'Mobile Development and UI/UX Design';
              } else if (lowerProg.includes('game') || lowerProg.includes('unity')) {
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
    await this.usersRepository.update(user.id, {
      googleId,
      avatarUrl: avatarUrl ?? undefined,
      status: UserStatus.ACTIVE,
      lastLoginAt: new Date(),
    });

    return (await this.findById(user.id))!;
  }

  async updateLastLogin(user: User): Promise<void> {
    await this.usersRepository.update(user.id, {
      lastLoginAt: new Date(),
    });
  }

  async suspend(id: string): Promise<User> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    user.status = UserStatus.SUSPENDED;
    return this.usersRepository.save(user);
  }

  async unsuspend(id: string): Promise<User> {
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
  async updateEmail(id: string, newEmail: string): Promise<User> {
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
      throw new ConflictException(`Email ${cleanedEmail} sudah terdaftar di sistem.`);
    }

    user.email = cleanedEmail;
    user.googleId = null; // Reset Google Auth binding
    user.status = UserStatus.INVITED; // Require re-verification/login

    const updatedUser = await this.usersRepository.save(user);

    return updatedUser;
  }

  async update(id: string, dto: UpdateUserDto): Promise<User> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    Object.assign(user, dto);
    return this.usersRepository.save(user);
  }

  async remove(id: string): Promise<void> {
    const user = await this.findById(id);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    // Clean up relational foreign keys before deleting user to prevent FK constraint error 23503
    await this.dataSource.query('UPDATE classes SET "mentorId" = NULL WHERE "mentorId" = $1', [id]);
    await this.dataSource.query('UPDATE competencies SET "creatorMentorId" = NULL WHERE "creatorMentorId" = $1', [id]);
    await this.dataSource.query('DELETE FROM enrollments WHERE "studentId" = $1', [id]);

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
      .filter((u) => u.role !== UserRole.ADMIN)
      .map((u) => u.id);

    if (nonAdminIds.length === 0) {
      return { deletedCount: 0 };
    }

    // Clean up relational foreign keys before bulk deleting users to prevent FK constraint error 23503
    await this.dataSource.query('UPDATE classes SET "mentorId" = NULL WHERE "mentorId" = ANY($1)', [nonAdminIds]);
    await this.dataSource.query('UPDATE competencies SET "creatorMentorId" = NULL WHERE "creatorMentorId" = ANY($1)', [nonAdminIds]);
    await this.dataSource.query('DELETE FROM enrollments WHERE "studentId" = ANY($1)', [nonAdminIds]);

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

