import { Injectable, NotFoundException, ForbiddenException, BadRequestException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, In, Not, IsNull } from 'typeorm';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity.js';
import { Material } from './entities/material.entity.js';
import { Assignment } from './entities/assignment.entity.js';
import { Competency } from './entities/competency.entity.js';
import { Program } from './entities/program.entity.js';
import { Batch, BatchStatus } from './entities/batch.entity.js';
import { User, UserRole, UserStatus } from '../users/entities/user.entity.js';
import { Submission } from './entities/submission.entity.js';
import { Logbook, LogbookStatus } from './entities/logbook.entity.js';

@Injectable()
export class ClassesService {
  constructor(
    @InjectRepository(Enrollment)
    private enrollmentRepository: Repository<Enrollment>,
    @InjectRepository(Class)
    private classRepository: Repository<Class>,
    @InjectRepository(Material)
    private materialRepository: Repository<Material>,
    @InjectRepository(Assignment)
    private assignmentRepository: Repository<Assignment>,
    @InjectRepository(Competency)
    private competencyRepository: Repository<Competency>,
    @InjectRepository(Program)
    private programRepository: Repository<Program>,
    @InjectRepository(Batch)
    private batchRepository: Repository<Batch>,
    @InjectRepository(User)
    private userRepository: Repository<User>,
    @InjectRepository(Submission)
    private submissionRepository: Repository<Submission>,
    @InjectRepository(Logbook)
    private logbookRepository: Repository<Logbook>,
  ) { }

  async healEnrollments() {
    try {
      const activeBatch = await this.batchRepository.findOne({ where: { status: BatchStatus.ACTIVE } });
      if (!activeBatch) return;

      const enrollments = await this.enrollmentRepository.find({
        relations: { class: true }
      });

      const mentors = await this.userRepository.find();
      const mentorUsers = mentors.filter(u => 
        u.roles?.includes(UserRole.MENTOR) || u.role === UserRole.MENTOR
      );

      for (const enroll of enrollments) {
        const cls = enroll.class;
        if (!cls || cls.batchId !== activeBatch.id) continue;

        // If the class has no mentor
        if (!cls.mentorId) {
          // 1. Check if there's already a class with a mentor for this program and batch
          const classWithMentor = await this.classRepository.findOne({
            where: {
              programId: cls.programId,
              batchId: cls.batchId,
              mentorId: Not(IsNull())
            }
          });

          if (classWithMentor) {
            await this.enrollmentRepository.update(enroll.id, { classId: classWithMentor.id });
            console.log(`[Heal] Moved student enrollment ${enroll.id} to class with mentor ${classWithMentor.mentorId}`);
          } else {
            // 2. Try to find a mentor user assigned to this program (active or invited)
            const program = await this.programRepository.findOne({ where: { id: cls.programId } });
            if (program) {
              const programMentor = mentorUsers.find(m => m.selectedProgram === program.name);
              if (programMentor) {
                // Create a class for this mentor
                let mentorClass = await this.classRepository.findOne({
                  where: {
                    programId: program.id,
                    batchId: activeBatch.id,
                    mentorId: programMentor.id
                  }
                });
                if (!mentorClass) {
                  mentorClass = await this.classRepository.save(this.classRepository.create({
                    programId: program.id,
                    batchId: activeBatch.id,
                    mentorId: programMentor.id
                  }));
                }
                await this.enrollmentRepository.update(enroll.id, { classId: mentorClass.id });
                console.log(`[Heal] Created class and moved student enrollment ${enroll.id} to mentor ${programMentor.name}`);
              }
            }
          }
        }
      }
    } catch (err) {
      console.error('[Heal] Error healing enrollments:', err);
    }
  }

  async findMyClasses(studentId: string) {
    await this.healEnrollments();
    const enrollments = await this.enrollmentRepository.find({
      where: { studentId },
      relations: {
        class: {
          program: true,
          batch: true,
          mentor: true
        }
      },
    });

    const enrichedClasses = await Promise.all(
      enrollments.map(async (e) => {
        const cls = e.class;
        const relatedClasses = await this.classRepository.find({
          where: { programId: cls.programId, batchId: cls.batchId }
        });
        const relatedClassIds = relatedClasses.map(c => c.id);

        const assignments = await this.assignmentRepository.find({
          where: { classId: In(relatedClassIds) },
          order: { createdAt: 'ASC' }
        });

        return {
          ...cls,
          assignments
        };
      })
    );

    return enrichedClasses;
  }

  async findMentorClasses(mentorId: string) {
    await this.healEnrollments();
    let classes = await this.classRepository.find({
      where: { mentorId },
      relations: {
        program: true,
        batch: true,
      },
    });

    // For each class, fetch enrollments to get student list
    const enrichedClasses = await Promise.all(
      classes.map(async (cls) => {
        const enrollments = await this.enrollmentRepository.find({
          where: { classId: cls.id },
          relations: { student: true },
        });
        const enrolledStudents = enrollments.map((e) => e.student).filter(Boolean);

        const relatedClasses = await this.classRepository.find({
          where: { programId: cls.programId, batchId: cls.batchId }
        });
        const relatedClassIds = relatedClasses.map(c => c.id);

        const materials = await this.materialRepository.find({
          where: { classId: In(relatedClassIds) }
        });
        const assignments = await this.assignmentRepository.find({
          where: { classId: In(relatedClassIds) },
          relations: { submissions: true },
        });

        return {
          ...cls,
          materials,
          assignments,
          enrolledStudentsCount: enrolledStudents.length,
          enrolledStudents,
        };
      }),
    );

    return enrichedClasses;
  }

  // ─── PROGRAM MANAGEMENT ENDPOINTS (ADMIN & MENTOR TAB) ───

  async getProgramsWithDetails() {
    await this.healEnrollments();
    const officialNames = [
      'AI Development',
      'Game Development',
      'Web Development and UI/UX Design',
      'Mobile Development and UI/UX Design'
    ];

    let programs = await this.programRepository.find();
    for (const name of officialNames) {
      if (!programs.some(p => p.name === name)) {
        const newProg = await this.programRepository.save(this.programRepository.create({
          name,
          description: `Program Resmi ${name} sesuai Source of Truth v2.0`
        }));
        programs.push(newProg);
      }
    }

    // Clean slate reset: Ensure legacy global batch is completed if it exists without programId
    let legacyBatch = await this.batchRepository.findOne({ where: { name: 'Batch 7 - 2026' } });
    if (legacyBatch && legacyBatch.status === BatchStatus.ACTIVE) {
      legacyBatch.status = BatchStatus.COMPLETED;
      await this.batchRepository.save(legacyBatch);
    }

    const allBatches = await this.batchRepository.find({ order: { createdAt: 'DESC' } });
    const allUsers = await this.userRepository.find();
    const allClasses = await this.classRepository.find();
    const allEnrollments = await this.enrollmentRepository.find();

    const result = await Promise.all(programs.map(async (prog) => {
      const classes = allClasses.filter(c => c.programId === prog.id);

      const progBatches = allBatches.filter(b => !b.includedProgramIds || b.includedProgramIds.length === 0 || b.includedProgramIds.includes(prog.id) || b.programId === prog.id);
      const activeBatch = allBatches.find(b => b.status === BatchStatus.ACTIVE && (!b.includedProgramIds || b.includedProgramIds.length === 0 || b.includedProgramIds.includes(prog.id) || b.programId === prog.id)) || null;
      const batchHistory = progBatches.filter(b => b.status !== BatchStatus.ACTIVE || b.id !== activeBatch?.id).map(b => ({
        id: b.id,
        name: b.name,
        status: b.status,
        createdAt: b.createdAt
      }));

      const activeBatchClasses = classes.filter(c => c.batchId === activeBatch?.id);
      const activeBatchClassIds = activeBatchClasses.map(c => c.id);

      const programMentors = allUsers.filter(u =>
        (u.roles.includes(UserRole.MENTOR) || u.roles.includes(UserRole.ADMIN)) &&
        u.status === UserStatus.ACTIVE &&
        activeBatch && activeBatchClasses.some(c => c.mentorId === u.id)
      );

      const programStudents = allUsers.filter(u => 
        u.roles.includes(UserRole.STUDENT) && 
        allEnrollments.some(e => e.studentId === u.id && activeBatchClassIds.includes(e.classId))
      );

      return {
        id: prog.id,
        name: prog.name,
        description: prog.description,
        activeBatch: activeBatch ? { id: activeBatch.id, name: activeBatch.name, status: activeBatch.status } : null,
        batch: activeBatch ? { id: activeBatch.id, name: activeBatch.name, status: activeBatch.status } : { id: 'no-batch', name: 'Belum Ada Batch Berjalan', status: 'completed' },
        batchHistory,
        mentorsCount: programMentors.length,
        mentors: programMentors.map(m => ({ id: m.id, name: m.name, email: m.email, whatsapp: m.whatsapp, status: m.status, specialization: m.specialization })),
        studentsCount: programStudents.length,
        students: programStudents.map(s => {
          const enroll = allEnrollments.find(e => e.studentId === s.id && activeBatchClassIds.includes(e.classId));
          const cls = allClasses.find(c => c.id === enroll?.classId);
          const mentor = programMentors.find(m => m.id === cls?.mentorId);
          return {
            id: s.id,
            name: s.name,
            email: s.email,
            whatsapp: s.whatsapp,
            status: s.status,
            selectedProgram: s.selectedProgram,
            mentorName: mentor ? mentor.name : 'Belum Ditentukan'
          };
        }),
      };
    }));

    const globalActive = allBatches.find(b => b.status === 'active') || { id: 'no-batch', name: 'Belum Ada Batch Berjalan', status: 'completed' };

    return {
      batch: { id: globalActive.id, name: globalActive.name, status: globalActive.status },
      programs: result,
      availableMentors: allUsers.filter(u => u.roles.includes(UserRole.MENTOR) && u.status === UserStatus.ACTIVE).map(m => ({
        id: m.id, name: m.name, email: m.email, whatsapp: m.whatsapp, status: m.status, selectedProgram: m.selectedProgram, specialization: m.specialization
      }))
    };
  }

  async updateBatchStatus(payload: { status: 'active' | 'completed'; batchId?: string; programId?: string } | 'active' | 'completed') {
    const rawStatus = typeof payload === 'string' ? payload : payload.status;
    const status = (rawStatus === 'active' ? BatchStatus.ACTIVE : BatchStatus.COMPLETED);
    const batchId = typeof payload === 'object' ? payload.batchId : undefined;
    const programId = typeof payload === 'object' ? payload.programId : undefined;

    if (batchId) {
      const b = await this.batchRepository.findOne({ where: { id: batchId } });
      if (b) {
        b.status = status;
        await this.batchRepository.save(b);
      }
    } else if (programId) {
      const batches = await this.batchRepository.find({ where: { programId, status: status === BatchStatus.ACTIVE ? BatchStatus.COMPLETED : BatchStatus.ACTIVE } });
      for (const b of batches) {
        b.status = status;
        await this.batchRepository.save(b);
      }
    } else {
      const activeBatches = await this.batchRepository.find({ where: { status: status === BatchStatus.ACTIVE ? BatchStatus.COMPLETED : BatchStatus.ACTIVE } });
      for (const b of activeBatches) {
        b.status = status;
        await this.batchRepository.save(b);
      }
    }
    return { success: true, status };
  }

  async getAllBatches() {
    const batches = await this.batchRepository.find({ order: { createdAt: 'DESC' } });
    const programs = await this.programRepository.find();
    const allClasses = await this.classRepository.find();
    const allEnrollments = await this.enrollmentRepository.find();
    const allUsers = await this.userRepository.find();

    return batches.map(batch => {
      const batchClasses = allClasses.filter(c => c.batchId === batch.id);
      const batchEnrollments = allEnrollments.filter(e => batchClasses.some(c => c.id === e.classId));

      const includedPrograms = batch.includedProgramIds && batch.includedProgramIds.length > 0
        ? programs.filter(p => batch.includedProgramIds?.includes(p.id))
        : programs;

      const programsWithDetails = includedPrograms.map(p => {
        const progClasses = batchClasses.filter(c => c.programId === p.id);
        const progEnrollments = batchEnrollments.filter(e => progClasses.some(c => c.id === e.classId));

        const progMentors = allUsers.filter(u =>
          (u.roles.includes(UserRole.MENTOR) || u.roles.includes(UserRole.ADMIN)) &&
          u.status === UserStatus.ACTIVE &&
          (progClasses.some(c => c.mentorId === u.id) || u.selectedProgram === p.name)
        );

        const progStudents = allUsers.filter(u =>
          u.roles.includes(UserRole.STUDENT) &&
          progEnrollments.some(e => e.studentId === u.id)
        );

        return {
          ...p,
          mentorsCount: progMentors.length,
          mentors: progMentors.map(m => ({
            id: m.id,
            name: m.name,
            email: m.email,
            whatsapp: m.whatsapp,
            status: m.status,
            specialization: m.specialization
          })),
          studentsCount: progStudents.length,
          students: progStudents.map(s => {
            const enroll = progEnrollments.find(e => e.studentId === s.id);
            const cls = progClasses.find(c => c.id === enroll?.classId);
            const mentor = progMentors.find(m => m.id === cls?.mentorId);
            return {
              id: s.id,
              name: s.name,
              email: s.email,
              whatsapp: s.whatsapp,
              status: s.status,
              selectedProgram: s.selectedProgram,
              mentorName: mentor ? mentor.name : (progMentors.map(m => m.name).join(', ') || 'Seluruh Mentor Program')
            };
          })
        };
      });

      return {
        ...batch,
        includedPrograms: programsWithDetails,
        classCount: batchClasses.length,
        studentCount: batchEnrollments.length,
      };
    });
  }

  async createGlobalBatch(payload: {
    name: string;
    status?: string;
    includedProgramIds?: string[];
    newProgramNames?: string[];
    startDate?: string | Date;
    endDate?: string | Date;
  }) {
    const status = (payload.status as BatchStatus) || BatchStatus.DRAFT;

    let programIds: string[] = payload.includedProgramIds ? [...payload.includedProgramIds] : [];

    if (payload.newProgramNames && payload.newProgramNames.length > 0) {
      for (const progName of payload.newProgramNames) {
        if (progName && progName.trim().length > 0) {
          let existingProg = await this.programRepository.findOne({ where: { name: progName.trim() } });
          if (!existingProg) {
            existingProg = await this.programRepository.save(this.programRepository.create({
              name: progName.trim(),
              description: `Program studi ekspansi kurikulum ${progName.trim()}`,
            }));
          }
          if (!programIds.includes(existingProg.id)) {
            programIds.push(existingProg.id);
          }
        }
      }
    }

    if (status === BatchStatus.ACTIVE) {
      const activeBatches = await this.batchRepository.find({ where: { status: BatchStatus.ACTIVE } });
      for (const b of activeBatches) {
        b.status = BatchStatus.COMPLETED;
        await this.batchRepository.save(b);
      }
    }

    const newBatch = (await this.batchRepository.save(this.batchRepository.create({
      name: payload.name,
      status,
      includedProgramIds: programIds,
      startDate: payload.startDate ? new Date(payload.startDate) : null,
      endDate: payload.endDate ? new Date(payload.endDate) : null,
    }))) as Batch;

    if (status === BatchStatus.ACTIVE) {
      for (const progId of programIds) {
        let cls = await this.classRepository.findOne({ where: { programId: progId, batchId: newBatch.id } });
        if (!cls) {
          await this.classRepository.save(this.classRepository.create({
            programId: progId,
            batchId: newBatch.id,
          }));
        }
      }
    }

    return { success: true, batch: newBatch, message: `Batch angkatan "${payload.name}" berhasil dibuat dengan status ${status.toUpperCase()}.` };
  }

  async updateGlobalBatch(batchId: string, payload: {
    name?: string;
    status?: string;
    includedProgramIds?: string[];
    newProgramNames?: string[];
    startDate?: string | Date;
    endDate?: string | Date;
  }) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) throw new NotFoundException('Batch tidak ditemukan');

    if (payload.name) batch.name = payload.name;
    if (payload.startDate !== undefined) batch.startDate = payload.startDate ? new Date(payload.startDate) : null;
    if (payload.endDate !== undefined) batch.endDate = payload.endDate ? new Date(payload.endDate) : null;

    if (payload.status) {
      const castStatus = payload.status as BatchStatus;
      if (castStatus === BatchStatus.ACTIVE && batch.status !== BatchStatus.ACTIVE) {
        const activeBatches = await this.batchRepository.find({ where: { status: BatchStatus.ACTIVE } });
        for (const b of activeBatches) {
          if (b.id !== batch.id) {
            b.status = BatchStatus.COMPLETED;
            await this.batchRepository.save(b);
          }
        }
      }
      batch.status = castStatus;
    }

    let programIds: string[] = payload.includedProgramIds ? [...payload.includedProgramIds] : (batch.includedProgramIds || []);

    if (payload.newProgramNames && payload.newProgramNames.length > 0) {
      for (const progName of payload.newProgramNames) {
        if (progName && progName.trim().length > 0) {
          let existingProg = await this.programRepository.findOne({ where: { name: progName.trim() } });
          if (!existingProg) {
            existingProg = await this.programRepository.save(this.programRepository.create({
              name: progName.trim(),
              description: `Program studi ekspansi kurikulum ${progName.trim()}`,
            }));
          }
          if (!programIds.includes(existingProg.id)) {
            programIds.push(existingProg.id);
          }
        }
      }
    }

    if (payload.includedProgramIds !== undefined || (payload.newProgramNames && payload.newProgramNames.length > 0)) {
      batch.includedProgramIds = programIds;
    }

    await this.batchRepository.save(batch);

    if (batch.status === 'active') {
      for (const progId of programIds) {
        let cls = await this.classRepository.findOne({ where: { programId: progId, batchId: batch.id } });
        if (!cls) {
          await this.classRepository.save(this.classRepository.create({
            programId: progId,
            batchId: batch.id,
          }));
        }
      }
    }

    return { success: true, batch, message: `Batch "${batch.name}" berhasil diperbarui.` };
  }

  async deleteGlobalBatch(batchId: string) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) throw new NotFoundException('Batch tidak ditemukan');

    const batchClasses = await this.classRepository.find({ where: { batchId: batch.id } });
    for (const cls of batchClasses) {
      const enrolls = await this.enrollmentRepository.count({ where: { classId: cls.id } });
      const mats = await this.materialRepository.count({ where: { classId: cls.id } });
      const ass = await this.assignmentRepository.count({ where: { classId: cls.id } });

      if (enrolls > 0 || mats > 0 || ass > 0) {
        if (batch.status === 'active' || batch.status === 'completed') {
          throw new ForbiddenException(`Batch "${batch.name}" tidak dapat dihapus karena sudah memiliki data kelas/murid aktif. Silakan ubah status menjadi Read-Only (Selesai).`);
        }
      }
      await this.enrollmentRepository.delete({ classId: cls.id });
      await this.materialRepository.delete({ classId: cls.id });
      await this.assignmentRepository.delete({ classId: cls.id });
      await this.classRepository.delete({ id: cls.id });
    }

    await this.batchRepository.delete({ id: batch.id });
    return { success: true, message: `Batch "${batch.name}" berhasil dihapus sepenuhnya.` };
  }

  async assignBatchMentors(batchId: string, programId: string, mentorIds: string[]) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) throw new NotFoundException('Batch tidak ditemukan');
    const program = await this.programRepository.findOne({ where: { id: programId } });
    if (!program) throw new NotFoundException('Program studi tidak ditemukan');

    const batchClasses = await this.classRepository.find({ where: { batchId: batch.id, programId: program.id } });
    const allMentors = await this.userRepository.find({ where: { role: UserRole.MENTOR } });

    // Ensure classes exist for selected mentors
    const otherClasses = await this.classRepository.find({ where: { batchId: batch.id } });

    for (const mId of mentorIds) {
      const m = allMentors.find(u => u.id === mId);
      const isUIUX = m?.specialization?.includes('UI/UX');
      const isProfessional = m?.specialization?.includes('Professional');

      if (!isUIUX && !isProfessional) {
        const existingInOther = otherClasses.find(c => c.mentorId === mId && c.programId !== program.id);
        if (existingInOther) {
          throw new BadRequestException(`Mentor Utama ${m?.name} tidak boleh ditugaskan di lebih dari 1 program!`);
        }
      } else if (isUIUX) {
        if (!program.name.includes('Web') && !program.name.includes('Mobile')) {
          throw new BadRequestException(`Mentor UI/UX ${m?.name} hanya boleh ditugaskan di Program Web atau Mobile!`);
        }
      }

      let cls = batchClasses.find(c => c.mentorId === mId);
      if (!cls) {
        cls = await this.classRepository.save(this.classRepository.create({
          batchId: batch.id,
          programId: program.id,
          mentorId: mId,
        }));
        batchClasses.push(cls);
      }
      if (m && m.selectedProgram !== program.name) {
        m.selectedProgram = program.name;
        await this.userRepository.save(m);
      }
    }

    // Remove or unassign mentors not in mentorIds for this program in this batch
    for (const cls of batchClasses) {
      if (cls.mentorId && !mentorIds.includes(cls.mentorId)) {
        const enrollCount = await this.enrollmentRepository.count({ where: { classId: cls.id } });
        if (enrollCount === 0 && batchClasses.length > 1) {
          await this.classRepository.delete({ id: cls.id });
        } else {
          cls.mentorId = null as any;
          await this.classRepository.save(cls);
        }
      }
    }

    return { success: true, message: `Berhasil memperbarui penugasan mentor untuk program ${program.name} di angkatan "${batch.name}".` };
  }

  async importAndEnrollBatch(batchId: string, payload: {
    users: Array<{
      name: string;
      email: string;
      whatsapp?: string;
      institution?: string;
      studyProgram?: string;
      selectedProgram: string;
    }>;
    autoDistribute?: boolean;
  }) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) throw new NotFoundException('Batch tidak ditemukan');

    const programs = await this.programRepository.find();
    let importedCount = 0;
    let enrolledCount = 0;
    const distributionSummary: Record<string, number> = {};

    for (const item of payload.users) {
      if (!item.email || !item.name) continue;

      const cleanEmail = item.email.trim().toLowerCase();
      const cleanName = item.name.trim();

      // 1. Silent Whitelist / User Creation (Rule: Gmail silent, no spam)
      let user = await this.userRepository.findOne({ where: { email: cleanEmail } });
      if (!user) {
        user = await this.userRepository.save(this.userRepository.create({
          email: cleanEmail,
          name: cleanName,
          role: UserRole.STUDENT,
          status: UserStatus.INVITED, // Silent whitelist per Gmail rule
          whatsapp: item.whatsapp ? item.whatsapp.replace(/\D/g, '') : undefined,
          institution: item.institution ? item.institution.trim() : undefined,
          studyProgram: item.studyProgram ? item.studyProgram.trim() : undefined,
          selectedProgram: item.selectedProgram ? item.selectedProgram.trim() : undefined,
        }));
        importedCount++;
      } else {
        if (item.whatsapp) user.whatsapp = item.whatsapp.replace(/\D/g, '');
        if (item.institution) user.institution = item.institution.trim();
        if (item.studyProgram) user.studyProgram = item.studyProgram.trim();
        if (item.selectedProgram) user.selectedProgram = item.selectedProgram.trim();
        await this.userRepository.save(user);
        importedCount++;
      }

      // 2. Auto-Enrollment into selectedProgram within target Batch
      if (user.selectedProgram) {
        let prog = programs.find(p => p.name.toLowerCase() === user.selectedProgram?.toLowerCase());
        if (!prog) {
          prog = await this.programRepository.save(this.programRepository.create({
            name: user.selectedProgram,
            description: `Program studi ${user.selectedProgram}`,
          }));
          programs.push(prog);

          if (!batch.includedProgramIds) batch.includedProgramIds = [];
          if (!batch.includedProgramIds.includes(prog.id)) {
            batch.includedProgramIds.push(prog.id);
            await this.batchRepository.save(batch);
          }
        }

        let cls = await this.classRepository.findOne({ where: { programId: prog.id, batchId: batch.id } });
        if (!cls) {
          cls = await this.classRepository.save(this.classRepository.create({
            programId: prog.id,
            batchId: batch.id,
          }));
        }

        const existingEnroll = await this.enrollmentRepository.findOne({ where: { studentId: user.id, classId: cls.id } });
        if (!existingEnroll) {
          await this.enrollmentRepository.save(this.enrollmentRepository.create({
            studentId: user.id,
            classId: cls.id,
          }));
          enrolledCount++;
          distributionSummary[prog.name] = (distributionSummary[prog.name] || 0) + 1;
        }
      }
    }

    // 3. Auto-Distribution across mentors for each program in this batch
    if (payload.autoDistribute) {
      const allMentors = await this.userRepository.find({ where: { role: UserRole.MENTOR, status: UserStatus.ACTIVE } });
      const batchClasses = await this.classRepository.find({ where: { batchId: batch.id } });
      const allEnrollments = await this.enrollmentRepository.find();

      for (const prog of programs) {
        const progMentors = allMentors.filter(m => m.selectedProgram === prog.name || batchClasses.some(c => c.programId === prog.id && c.mentorId === m.id));
        if (progMentors.length > 0) {
          const progClasses = batchClasses.filter(c => c.programId === prog.id);
          const progEnrollments = allEnrollments.filter(e => progClasses.some(c => c.id === e.classId));

          const mentorClasses: Class[] = [];
          for (const m of progMentors) {
            let mCls = progClasses.find(c => c.mentorId === m.id);
            if (!mCls) {
              mCls = await this.classRepository.save(this.classRepository.create({
                programId: prog.id,
                batchId: batch.id,
                mentorId: m.id,
              }));
              progClasses.push(mCls);
            }
            mentorClasses.push(mCls);
          }

          progEnrollments.forEach((enroll, idx) => {
            const targetClass = mentorClasses[idx % mentorClasses.length];
            if (enroll.classId !== targetClass.id) {
              enroll.classId = targetClass.id;
              this.enrollmentRepository.save(enroll);
            }
          });
        }
      }
    }

    return {
      success: true,
      totalImported: importedCount,
      totalEnrolled: enrolledCount,
      distributionSummary,
      message: `Berhasil memproses ${importedCount} data murid: ${enrolledCount} terdaftar ke program studi dan didistribusikan.`,
    };
  }

  async createProgramBatch(payload: { programId: string; batchName: string }) {
    const prog = await this.programRepository.findOne({ where: { id: payload.programId } });
    if (!prog) throw new NotFoundException('Program studi tidak ditemukan');

    // Archive any existing active batch for this program
    const existingActive = await this.batchRepository.find({ where: { programId: prog.id, status: BatchStatus.ACTIVE } });
    for (const b of existingActive) {
      b.status = BatchStatus.COMPLETED;
      await this.batchRepository.save(b);
    }

    // Create new active batch
    const newBatch = (await this.batchRepository.save(this.batchRepository.create({
      name: payload.batchName,
      programId: prog.id,
      status: BatchStatus.ACTIVE
    }))) as Batch;

    // Create or update class for this program to point to the new batch
    let cls = await this.classRepository.findOne({ where: { programId: prog.id } });
    if (!cls) {
      cls = this.classRepository.create({
        programId: prog.id,
        batchId: newBatch.id,
      });
    } else {
      cls.batchId = newBatch.id;
    }
    await this.classRepository.save(cls);

    return { success: true, batch: newBatch, message: `Batch baru "${payload.batchName}" berhasil dibuat untuk program ${prog.name}` };
  }

  async enrollStudentToProgram(payload: {
    studentId: string;
    programName: string;
    mentorId?: string;
    isCase3Transfer?: boolean;
  }) {
    // Validate program exists first
    const program = await this.programRepository.findOne({ where: { name: payload.programName } });
    if (!program) {
      throw new BadRequestException(`Program studi "${payload.programName}" tidak ditemukan di database.`);
    }

    // Validate active batch exists BEFORE any state mutation (Rule 27 + Entitas 5)
    const batch = await this.batchRepository.findOne({ where: { status: BatchStatus.ACTIVE } });
    if (!batch) {
      throw new BadRequestException('Tidak ada Batch/Cohort aktif. Silakan buat atau aktifkan Batch terlebih dahulu sebelum mendaftarkan siswa.');
    }

    const student = await this.userRepository.findOne({ where: { id: payload.studentId } });
    if (!student) throw new NotFoundException('Siswa tidak ditemukan');

    const oldProgram = student.selectedProgram;

    // Rule 25: CASE 3 CLEAN TRANSFER & PROGRESS RESET
    if (oldProgram && oldProgram !== payload.programName && payload.isCase3Transfer) {
      await this.enrollmentRepository.delete({ studentId: student.id });
      console.log(`[Clean Transfer] Removed all enrollments and progress for student ${student.email} from ${oldProgram}`);
    }

    student.selectedProgram = payload.programName;
    await this.userRepository.save(student);

    if (!batch.includedProgramIds) batch.includedProgramIds = [];
    if (!batch.includedProgramIds.includes(program.id)) {
      batch.includedProgramIds.push(program.id);
      await this.batchRepository.save(batch);
    }

    let mentorIdToUse = payload.mentorId;
    if (!mentorIdToUse) {
      // 1. Try to find an existing class with a mentor for this program in the active batch
      const classWithMentor = await this.classRepository.findOne({
        where: {
          programId: program.id,
          batchId: batch.id,
          mentorId: Not(IsNull())
        }
      });
      if (classWithMentor?.mentorId) {
        mentorIdToUse = classWithMentor.mentorId;
      } else {
        // 2. Try to find any mentor user assigned to this program (active or invited status!)
        const mentors = await this.userRepository.find();
        const anyMentor = mentors.find(u => 
          (u.roles?.includes(UserRole.MENTOR) || u.role === UserRole.MENTOR) && 
          u.selectedProgram === payload.programName
        );
        mentorIdToUse = anyMentor?.id;
      }
    }

    if (mentorIdToUse) {
      const mentor = await this.userRepository.findOne({ where: { id: mentorIdToUse } });
      if (mentor) {
        const spec = mentor.specialization || '';
        if (payload.programName.includes('AI')) {
          if (!spec.includes('AI')) {
            throw new BadRequestException(`Mentor ${mentor.name} dengan spesialisasi ${spec || 'kosong'} tidak diizinkan membimbing murid di program AI (Rule 1).`);
          }
        } else if (payload.programName.includes('Game')) {
          if (!spec.includes('Game')) {
            throw new BadRequestException(`Mentor ${mentor.name} dengan spesialisasi ${spec || 'kosong'} tidak diizinkan membimbing murid di program Game (Rule 2).`);
          }
        } else if (payload.programName.includes('Web')) {
          if (!spec.includes('Web') && !spec.includes('UI/UX') && !spec.includes('Professional')) {
            throw new BadRequestException(`Mentor ${mentor.name} dengan spesialisasi ${spec || 'kosong'} tidak diizinkan membimbing murid di program Web (Rule 3).`);
          }
        } else if (payload.programName.includes('Mobile')) {
          if (!spec.includes('Mobile') && !spec.includes('UI/UX') && !spec.includes('Professional')) {
            throw new BadRequestException(`Mentor ${mentor.name} dengan spesialisasi ${spec || 'kosong'} tidak diizinkan membimbing murid di program Mobile (Rule 4).`);
          }
        }

        if (!mentor.selectedProgram) {
          mentor.selectedProgram = payload.programName;
          await this.userRepository.save(mentor);
        }
      }
    }

    let cls = await this.classRepository.findOne({
      where: {
        programId: program.id,
        batchId: batch.id,
        mentorId: mentorIdToUse || IsNull()
      }
    });
    if (!cls) {
      cls = await this.classRepository.save(this.classRepository.create({
        programId: program.id,
        batchId: batch.id,
        mentorId: mentorIdToUse || null,
      }));
    }

    // Ensure exactly 1 personal mentor by deleting any existing enrollments in this program
    const allProgramClasses = await this.classRepository.find({ where: { programId: program.id } });
    const programClassIds = allProgramClasses.map(c => c.id);

    const existingEnrolls = await this.enrollmentRepository.find({ where: { studentId: student.id } });
    for (const e of existingEnrolls) {
      if (programClassIds.includes(e.classId)) {
        await this.enrollmentRepository.delete({ id: e.id });
      }
    }

    await this.enrollmentRepository.save(this.enrollmentRepository.create({
      studentId: student.id,
      classId: cls.id,
    }));

    return { success: true, student };
  }

  async assignMentorToProgram(mentorId: string, programName: string) {
    // Validate program exists first
    const program = await this.programRepository.findOne({ where: { name: programName } });
    if (!program) {
      throw new BadRequestException(`Program studi "${programName}" tidak ditemukan di database.`);
    }

    // Validate active batch exists BEFORE any state mutation
    const batch = await this.batchRepository.findOne({ where: { status: BatchStatus.ACTIVE } });
    if (!batch) {
      throw new BadRequestException('Tidak ada Batch/Cohort aktif. Silakan buat atau aktifkan Batch terlebih dahulu sebelum meng-assign mentor.');
    }

    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor) throw new NotFoundException('Mentor tidak ditemukan');

    mentor.selectedProgram = programName;
    await this.userRepository.save(mentor);

    if (!batch.includedProgramIds) batch.includedProgramIds = [];
    if (!batch.includedProgramIds.includes(program.id)) {
      batch.includedProgramIds.push(program.id);
      await this.batchRepository.save(batch);
    }

    let cls = await this.classRepository.findOne({ where: { programId: program.id, mentorId: mentor.id } });
    if (!cls) {
      cls = await this.classRepository.save(this.classRepository.create({
        programId: program.id,
        batchId: batch.id,
        mentorId: mentor.id,
      }));
    }

    // Move any students currently in a mentor-less class for this program/batch to this mentor's class
    const mentorlessClass = await this.classRepository.findOne({
      where: {
        programId: program.id,
        batchId: batch.id,
        mentorId: IsNull()
      }
    });

    if (mentorlessClass) {
      const mentorlessEnrollments = await this.enrollmentRepository.find({
        where: { classId: mentorlessClass.id }
      });
      for (const enroll of mentorlessEnrollments) {
        await this.enrollmentRepository.update(enroll.id, { classId: cls.id });
        console.log(`[Assign Mentor] Moved student enrollment ${enroll.id} to new mentor ${mentor.name}`);
      }
    }

    return { success: true, mentor };
  }

  async distributeModulo(programName: string) {
    const activeBatch = await this.batchRepository.findOne({ where: { status: BatchStatus.ACTIVE } });
    if (!activeBatch) {
      throw new BadRequestException('Tidak ada Batch/Cohort yang sedang aktif.');
    }

    const program = await this.programRepository.findOne({ where: { name: programName } });
    if (!program) {
      throw new NotFoundException('Program studi tidak ditemukan');
    }

    const allUsers = await this.userRepository.find();
    const programStudents = allUsers.filter(u => u.roles.includes(UserRole.STUDENT) && u.selectedProgram === programName);

    const batchClasses = await this.classRepository.find({ where: { batchId: activeBatch.id, programId: program.id } });
    const batchClassIds = batchClasses.map(c => c.id);
    const enrollments = await this.enrollmentRepository.find({
      where: { classId: In(batchClassIds) }
    });

    const studentsInBatch = programStudents.filter(s => enrollments.some(e => e.studentId === s.id));

    const primaryMentors = allUsers.filter(u =>
      u.roles.includes(UserRole.MENTOR) &&
      u.status === UserStatus.ACTIVE &&
      u.selectedProgram === programName &&
      !u.specialization?.includes('UI/UX') &&
      !u.specialization?.includes('Professional')
    );

    const secondaryMentors = allUsers.filter(u =>
      u.roles.includes(UserRole.MENTOR) &&
      u.status === UserStatus.ACTIVE &&
      u.specialization?.includes('UI/UX')
    );
    const supportingMentors = allUsers.filter(u =>
      u.roles.includes(UserRole.MENTOR) &&
      u.status === UserStatus.ACTIVE &&
      u.specialization?.includes('Professional')
    );

    const totalStudents = studentsInBatch.length;
    const numPrimary = Math.max(1, primaryMentors.length);
    const baseAllocation = Math.floor(totalStudents / numPrimary);
    const remainder = totalStudents % numPrimary;

    if (totalStudents === 0) {
      throw new BadRequestException('Tidak ada siswa aktif yang terdaftar di batch berjalan pada program ini.');
    }

    const getOrCreateClass = async (mentorId: string) => {
      let cls = batchClasses.find(c => c.mentorId === mentorId);
      if (!cls) {
        cls = await this.classRepository.save(this.classRepository.create({
          batchId: activeBatch.id,
          programId: program.id,
          mentorId,
        }));
        batchClasses.push(cls);
      }
      return cls;
    };

    const primaryClasses: Class[] = [];
    for (const m of primaryMentors) {
      primaryClasses.push(await getOrCreateClass(m.id));
    }

    const supportMentors = [...secondaryMentors, ...supportingMentors];
    const supportClasses: Class[] = [];
    for (const m of supportMentors) {
      supportClasses.push(await getOrCreateClass(m.id));
    }

    let studentIdx = 0;
    for (let mIdx = 0; mIdx < primaryClasses.length; mIdx++) {
      const cls = primaryClasses[mIdx];
      for (let i = 0; i < baseAllocation; i++) {
        if (studentIdx >= studentsInBatch.length) break;
        const student = studentsInBatch[studentIdx++];
        const enroll = enrollments.find(e => e.studentId === student.id);
        if (enroll) {
          enroll.classId = cls.id;
          await this.enrollmentRepository.save(enroll);
        }
      }
    }

    const isWebOrMobile = programName.includes('Web') || programName.includes('Mobile');
    if (isWebOrMobile && supportClasses.length > 0) {
      for (let i = 0; i < remainder; i++) {
        if (studentIdx >= studentsInBatch.length) break;
        const student = studentsInBatch[studentIdx++];
        const cls = supportClasses[i % supportClasses.length];
        const enroll = enrollments.find(e => e.studentId === student.id);
        if (enroll) {
          enroll.classId = cls.id;
          await this.enrollmentRepository.save(enroll);
        }
      }
    } else {
      for (let i = 0; i < remainder; i++) {
        if (studentIdx >= studentsInBatch.length) break;
        const student = studentsInBatch[studentIdx++];
        const cls = primaryClasses[i % primaryClasses.length];
        const enroll = enrollments.find(e => e.studentId === student.id);
        if (enroll) {
          enroll.classId = cls.id;
          await this.enrollmentRepository.save(enroll);
        }
      }
    }

    return {
      programName,
      totalStudents,
      numPrimaryMentors: primaryMentors.length,
      baseAllocationPerPrimary: baseAllocation,
      remainderModulo: remainder,
      secondaryUiUxMentorsCount: secondaryMentors.length,
      supportingProfessionalMentorsCount: supportingMentors.length,
      message: `Berhasil mengeksekusi kalkulasi Modulo: ${baseAllocation} murid per Mentor Utama. Sisa ${remainder} murid didistribusikan ke Mentor UI/UX dan Professional (untuk Web/Mobile) atau dibagikan ke Mentor Utama (untuk AI/Game).`
    };
  }

  async getCompetencies(programId?: string) {
    const whereClause = programId ? { programId } : {};
    const competencies = await this.competencyRepository.find({
      where: whereClause,
      relations: { program: true, creatorMentor: true },
    });

    return competencies;
  }

  private validateCompetencyAuthor(programName: string, specialization: string | null, category: string) {
    if (!specialization) return false;

    // Rule 9: Soft Skills (CCA) -> Mentor Professional
    if (category === 'Soft Skills (CCA)') {
      return specialization.includes('Professional');
    }

    // Rule 10: Capstone Project -> Mentor UI/UX
    if (category === 'Capstone Project') {
      return specialization.includes('UI/UX');
    }

    // Category Technical
    const progLower = programName.toLowerCase();
    const specLower = specialization.toLowerCase();

    // Rule 5 & 6: AI and Game are exclusive to their respective mentors
    if (progLower.includes('ai')) return specLower.includes('ai');
    if (progLower.includes('game')) return specLower.includes('game');

    // Rule 7 & 8: Web and Mobile can be Mentor Web/Mobile or Mentor UI/UX (UI/UX handles Capstone usually, but Rule 7/8 says dipegang bersama untuk Web/Mobile)
    if (progLower.includes('web')) return specLower.includes('web') || specLower.includes('ui/ux');
    if (progLower.includes('mobile')) return specLower.includes('mobile') || specLower.includes('ui/ux');

    return false;
  }

  async createCompetency(mentorId: string, payload: { name: string; category: string; programId: string; phase?: string }) {
    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor || !mentor.roles.includes(UserRole.MENTOR)) throw new ForbiddenException('Hanya mentor yang dapat membuat kompetensi.');

    const program = await this.programRepository.findOne({ where: { id: payload.programId } });
    if (!program) throw new NotFoundException('Program tidak ditemukan.');

    if (!this.validateCompetencyAuthor(program.name, mentor.specialization, payload.category)) {
      throw new ForbiddenException(`Mentor dengan spesialisasi ${mentor.specialization || 'Umum'} tidak berwenang membuat kompetensi kategori ${payload.category} untuk program ini.`);
    }

    const competency = this.competencyRepository.create({
      name: payload.name,
      category: payload.category,
      phase: payload.phase || 'Micro',
      programId: program.id,
      creatorMentorId: mentor.id,
    });
    return await this.competencyRepository.save(competency);
  }

  async updateCompetency(mentorId: string, id: string, payload: { name?: string; category?: string; phase?: string }) {
    const competency = await this.competencyRepository.findOne({ where: { id } });
    if (!competency) throw new NotFoundException('Kompetensi tidak ditemukan.');

    // Only creators or admins can edit (simplified rule for mentor)
    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor || !mentor.roles.includes(UserRole.MENTOR)) throw new ForbiddenException('Akses ditolak.');

    if (payload.name) competency.name = payload.name;
    if (payload.category) competency.category = payload.category;
    if (payload.phase) competency.phase = payload.phase;

    return await this.competencyRepository.save(competency);
  }

  async deleteCompetency(mentorId: string, id: string) {
    const competency = await this.competencyRepository.findOne({ where: { id } });
    if (!competency) throw new NotFoundException('Kompetensi tidak ditemukan.');

    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor || !mentor.roles.includes(UserRole.MENTOR)) throw new ForbiddenException('Akses ditolak.');

    // Check if there are assignments using this competency
    const assignmentsUsingIt = await this.assignmentRepository.count({ where: { competency: id } });
    if (assignmentsUsingIt > 0) {
      throw new ForbiddenException('Tidak dapat menghapus kompetensi karena masih ada tugas yang tertaut.');
    }

    await this.competencyRepository.remove(competency);
    return { success: true };
  }

  async createMaterial(mentorId: string, classId: string, payload: { title: string; type: string; competency: string; url: string; content?: string }) {
    const cls = await this.classRepository.findOne({ where: { id: classId }, relations: { batch: true, program: true } });
    if (!cls) throw new NotFoundException('Kelas tidak ditemukan.');

    // Rule 23: Batch Read Only
    if (cls.batch?.status === 'completed') throw new ForbiddenException('Batch sudah selesai (Read-Only Mode). Modifikasi tidak diizinkan.');

    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor) throw new ForbiddenException('Mentor tidak ditemukan.');

    // Validate if mentor is assigned to this class
    if (cls.mentorId !== mentorId) throw new ForbiddenException('Anda bukan mentor untuk kelas ini.');

    // We skip deep competency validation here assuming the UI filters it properly, but we could re-validate based on category if needed.

    const material = this.materialRepository.create({
      classId: cls.id,
      title: payload.title,
      type: payload.type,
      competency: payload.competency,
      url: payload.url,
      content: payload.content,
    });
    return await this.materialRepository.save(material);
  }

  async createAssignment(mentorId: string, classId: string, payload: { title: string; description: string; competency: string; dueDate: string }) {
    const cls = await this.classRepository.findOne({ where: { id: classId }, relations: { batch: true, program: true } });
    if (!cls) throw new NotFoundException('Kelas tidak ditemukan.');

    // Rule 23: Batch Read Only
    if (cls.batch?.status === 'completed') throw new ForbiddenException('Batch sudah selesai (Read-Only Mode). Modifikasi tidak diizinkan.');

    const mentor = await this.userRepository.findOne({ where: { id: mentorId } });
    if (!mentor) throw new ForbiddenException('Mentor tidak ditemukan.');

    if (cls.mentorId !== mentorId) throw new ForbiddenException('Anda bukan mentor untuk kelas ini.');

    const assignment = this.assignmentRepository.create({
      classId: cls.id,
      title: payload.title,
      description: payload.description,
      competency: payload.competency,
      dueDate: new Date(payload.dueDate),
    });
    return await this.assignmentRepository.save(assignment);
  }


  async getClassDetails(classId: string) {
    let classEntity: Class | null = null;

    // Check if classId is a valid UUID format before querying to prevent Postgres error
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (uuidRegex.test(classId)) {
      classEntity = await this.classRepository.findOne({
        where: { id: classId },
        relations: {
          program: true,
          batch: true,
          mentor: true,
        },
      });
    }

    if (!classEntity) {
      throw new NotFoundException('Kelas tidak ditemukan');
    }

    const relatedClasses = await this.classRepository.find({
      where: { programId: classEntity.programId, batchId: classEntity.batchId }
    });
    const relatedClassIds = relatedClasses.map(c => c.id);

    classEntity.materials = await this.materialRepository.find({
      where: { classId: In(relatedClassIds) },
      order: { createdAt: 'ASC' }
    });
    classEntity.assignments = await this.assignmentRepository.find({
      where: { classId: In(relatedClassIds) },
      order: { createdAt: 'ASC' }
    });

    return classEntity;
  }

  async getMaterialDetails(classId: string, materialId: string) {
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    let materialEntity: Material | null = null;

    if (uuidRegex.test(materialId) && uuidRegex.test(classId)) {
      const cls = await this.classRepository.findOne({ where: { id: classId } });
      if (cls) {
        const relatedClasses = await this.classRepository.find({
          where: { programId: cls.programId, batchId: cls.batchId }
        });
        const relatedClassIds = relatedClasses.map(c => c.id);

        materialEntity = await this.materialRepository.findOne({
          where: { id: materialId, classId: In(relatedClassIds) },
        });
      }
    }

    if (!materialEntity) {
      throw new NotFoundException('Materi tidak ditemukan');
    }

    return materialEntity;
  }

  async getAssignmentDetails(classId: string, assignmentId: string) {
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    let assignmentEntity: Assignment | null = null;

    if (uuidRegex.test(assignmentId) && uuidRegex.test(classId)) {
      const cls = await this.classRepository.findOne({ where: { id: classId } });
      if (cls) {
        const relatedClasses = await this.classRepository.find({
          where: { programId: cls.programId, batchId: cls.batchId }
        });
        const relatedClassIds = relatedClasses.map(c => c.id);

        assignmentEntity = await this.assignmentRepository.findOne({
          where: { id: assignmentId, classId: In(relatedClassIds) },
        });
      }
    }

    if (!assignmentEntity) {
      throw new NotFoundException('Tugas tidak ditemukan');
    }

    return assignmentEntity;
  }

  async handoverMentor(oldMentorId: string, newMentorId: string, programId: string) {
    const oldMentor = await this.userRepository.findOne({ where: { id: oldMentorId } });
    const newMentor = await this.userRepository.findOne({ where: { id: newMentorId } });
    if (!oldMentor || !newMentor) {
      throw new NotFoundException('Mentor lama atau mentor baru tidak ditemukan');
    }
    const program = await this.programRepository.findOne({ where: { id: programId } });
    if (!program) {
      throw new NotFoundException('Program studi tidak ditemukan');
    }

    if (!this.validateCompetencyAuthor(program.name, newMentor.specialization, 'Technical')) {
      throw new BadRequestException(`Mentor baru dengan spesialisasi ${newMentor.specialization || 'Umum'} tidak kompatibel dengan program ${program.name}`);
    }

    const activeBatch = await this.batchRepository.findOne({ where: { status: BatchStatus.ACTIVE } });
    if (!activeBatch) {
      throw new BadRequestException('Tidak ada Batch/Cohort yang sedang aktif.');
    }

    const oldClasses = await this.classRepository.find({
      where: { mentorId: oldMentorId, programId, batchId: activeBatch.id }
    });

    if (oldClasses.length === 0) {
      throw new BadRequestException('Mentor lama tidak memiliki kelas aktif pada program ini di Batch berjalan.');
    }

    let transferCount = 0;
    for (const cls of oldClasses) {
      let newCls = await this.classRepository.findOne({
        where: { mentorId: newMentorId, programId, batchId: activeBatch.id }
      });
      if (!newCls) {
        cls.mentorId = newMentorId;
        await this.classRepository.save(cls);
        transferCount++;
      } else {
        const enrolls = await this.enrollmentRepository.find({ where: { classId: cls.id } });
        for (const e of enrolls) {
          const exist = await this.enrollmentRepository.findOne({ where: { studentId: e.studentId, classId: newCls.id } });
          if (!exist) {
            e.classId = newCls.id;
            await this.enrollmentRepository.save(e);
          } else {
            await this.enrollmentRepository.delete({ id: e.id });
          }
        }
        await this.classRepository.delete({ id: cls.id });
        transferCount++;
      }
    }

    return {
      success: true,
      message: `Berhasil melakukan handover tugas bimbingan dari ${oldMentor.name} ke ${newMentor.name} untuk program ${program.name}. Sebanyak ${transferCount} kelas dialihkan.`
    };
  }

  async updateUserBatches(userId: string, batchIds: string[]) {
    const user = await this.userRepository.findOne({ where: { id: userId } });
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    const isStudent = user.roles.includes(UserRole.STUDENT);
    const isMentor = user.roles.includes(UserRole.MENTOR);

    if (!isStudent && !isMentor) {
      throw new BadRequestException('Hanya Student dan Mentor yang dapat dikaitkan dengan Cohort/Batch.');
    }

    if (isStudent) {
      if (!user.selectedProgram) {
        throw new BadRequestException('Siswa wajib memiliki program studi sebelum dikaitkan dengan Batch.');
      }
      const program = await this.programRepository.findOne({ where: { name: user.selectedProgram } });
      if (!program) {
        throw new BadRequestException(`Program studi "${user.selectedProgram}" tidak ditemukan.`);
      }

      // Get current enrollments
      const currentEnrollments = await this.enrollmentRepository.find({
        where: { studentId: userId },
        relations: { class: true }
      });

      // 1. Remove enrollments that are NOT in the list of batchIds
      for (const e of currentEnrollments) {
        if (!batchIds.includes(e.class.batchId)) {
          await this.enrollmentRepository.delete({ id: e.id });
        }
      }

      // 2. Add new enrollments for target batches
      for (const bId of batchIds) {
        const hasEnroll = currentEnrollments.some(e => e.class.batchId === bId);
        if (!hasEnroll) {
          // Find or create class for this program and batch
          let cls = await this.classRepository.findOne({ where: { programId: program.id, batchId: bId } });
          if (!cls) {
            // Find an active mentor for this program to set as initial mentor, if any
            const activeMentors = await this.userRepository.find({ where: { status: UserStatus.ACTIVE } });
            const anyMentor = activeMentors.find(u => u.roles.includes(UserRole.MENTOR) && u.selectedProgram === user.selectedProgram);
            cls = await this.classRepository.save(this.classRepository.create({
              programId: program.id,
              batchId: bId,
              mentorId: anyMentor?.id || null,
            }));
          }

          await this.enrollmentRepository.save(this.enrollmentRepository.create({
            studentId: userId,
            classId: cls.id,
          }));
        }
      }
    }

    if (isMentor) {
      if (!user.selectedProgram) {
        throw new BadRequestException('Mentor wajib memiliki program studi sebelum dikaitkan dengan Batch.');
      }
      const program = await this.programRepository.findOne({ where: { name: user.selectedProgram } });
      if (!program) {
        throw new BadRequestException(`Program studi "${user.selectedProgram}" tidak ditemukan.`);
      }

      // Find all classes assigned to this mentor
      const currentClasses = await this.classRepository.find({
        where: { mentorId: userId }
      });

      // 1. Unassign from classes that are NOT in batchIds
      for (const cls of currentClasses) {
        if (!batchIds.includes(cls.batchId)) {
          cls.mentorId = null;
          await this.classRepository.save(cls);
        }
      }

      // 2. Assign to classes in target batchIds
      for (const bId of batchIds) {
        const isAssigned = currentClasses.some(c => c.batchId === bId);
        if (!isAssigned) {
          let cls = await this.classRepository.findOne({ where: { programId: program.id, batchId: bId } });
          if (cls) {
            cls.mentorId = userId;
            await this.classRepository.save(cls);
          } else {
            await this.classRepository.save(this.classRepository.create({
              programId: program.id,
              batchId: bId,
              mentorId: userId
            }));
          }
        }
      }
    }

    return { success: true };
  }

  async updateCompetencyRubric(competencyId: string, rubric: any) {
    const competency = await this.competencyRepository.findOne({
      where: { id: competencyId },
    });

    if (!competency) {
      throw new NotFoundException('Competency not found');
    }

    competency.rubric = rubric;
    await this.competencyRepository.save(competency);

    return {
      success: true,
      message: 'Rubric updated successfully',
      rubric: competency.rubric,
    };
  }

  async submitAssignment(studentId: string, assignmentId: string, link: string) {
    let submission = await this.submissionRepository.findOne({
      where: { studentId, assignmentId },
    });
    if (!submission) {
      submission = this.submissionRepository.create({ studentId, assignmentId });
    }
    submission.link = link;
    submission.status = 'submitted';
    await this.submissionRepository.save(submission);
    return { success: true, submission };
  }

  async getStudentSubmission(studentId: string, assignmentId: string) {
    return this.submissionRepository.findOne({
      where: { studentId, assignmentId },
      relations: { assignment: true }
    });
  }

  async getSubmissions(assignmentId: string) {
    return this.submissionRepository.find({
      where: { assignmentId },
      relations: { student: true },
    });
  }

  async gradeSubmission(submissionId: string, mentorId: string, score: number, manualFeedback: string) {
    const submission = await this.submissionRepository.findOne({
      where: { id: submissionId },
      relations: { assignment: true }
    });
    if (!submission) throw new NotFoundException('Submission not found');

    let finalScore = score;
    let feedback = manualFeedback;

    // Check if late (createdAt > assignment.dueDate)
    if (submission.assignment && submission.assignment.dueDate) {
      const dueDate = new Date(submission.assignment.dueDate);
      const submittedAt = new Date(submission.createdAt);
      if (submittedAt > dueDate) {
        finalScore = Math.max(0, finalScore - 2);
        const lateMsg = '[Sistem] Mentee mengumpulkan terlambat. Nilai dikurangi 2 poin secara otomatis.';
        feedback = feedback ? feedback + '\n\n' + lateMsg : lateMsg;
      }
    }

    submission.score = finalScore;
    submission.manualFeedback = feedback;
    submission.gradedByMentorId = mentorId;
    submission.status = 'graded';
    await this.submissionRepository.save(submission);
    return { success: true, submission };
  }

  async aiEvaluateSubmission(submissionId: string) {
    const submission = await this.submissionRepository.findOne({
      where: { id: submissionId },
      relations: { assignment: { class: { program: true } } },
    });
    if (!submission) throw new NotFoundException('Submission not found');

    const assignment = submission.assignment;
    if (!assignment) throw new NotFoundException('Assignment missing');

    let rubric = null;
    if (assignment.competency) {
      const competencyObj = await this.competencyRepository.findOne({ where: { name: assignment.competency, program: { id: assignment.class.program.id } } });
      if (competencyObj) rubric = competencyObj.rubric;
    }

    try {
      const prompt = `Please evaluate the following submission link based on the rubric provided. This is an automated assessment prompt. 
Link: ${submission.link}
Rubric: ${JSON.stringify(rubric)}
Return a JSON object with 'score' (0-100) and 'feedback'.`;

      const response = await fetch('http://127.0.0.1:11434/api/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: 'llama3', // adjust model if needed
          prompt: prompt,
          stream: false,
          format: 'json',
        }),
      });

      if (!response.ok) {
        console.error('Ollama response not ok:', response.statusText);
        throw new Error('Ollama request failed');
      }

      const data = await response.json();
      const parsed = JSON.parse(data.response);
      submission.score = parsed.score;
      submission.aiFeedback = parsed.feedback;
      submission.status = 'ai_draft';
      await this.submissionRepository.save(submission);
      return { success: true, submission };
    } catch (e) {
      console.error('AI evaluation failed', e);
      // Fallback draft if ollama is not reachable or fails
      submission.score = 75;
      submission.aiFeedback = "Evaluasi AI gagal atau server AI tidak aktif. Ini adalah nilai draft fallback.";
      submission.status = 'ai_draft';
      await this.submissionRepository.save(submission);
      return { success: true, submission, message: 'AI fallback used' };
    }
  }

  async updateAssignmentWeights(updates: Array<{ id: string; weight: number }>) {
    for (const update of updates) {
      await this.assignmentRepository.update({ id: update.id }, { weight: update.weight });
    }
    return { success: true, message: 'Weights updated' };
  }
  // ================= LOGBOOK BULANAN =================
  async getStudentLogbooks(studentId: string, batchId: string) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) throw new NotFoundException('Batch not found');
    
    // Auto-calculate total months from batch startDate and endDate
    let totalMonths = 0;
    if (batch.startDate && batch.endDate) {
      const start = new Date(batch.startDate);
      const end = new Date(batch.endDate);
      totalMonths = (end.getFullYear() - start.getFullYear()) * 12 + (end.getMonth() - start.getMonth()) + 1;
    }
    
    if (totalMonths <= 0) totalMonths = 1; // Fallback to at least 1 month if misconfigured

    const logbooks = await this.logbookRepository.find({
      where: { studentId, batchId },
      order: { monthIndex: 'ASC' }
    });

    return { totalMonths, startDate: batch.startDate, logbooks };
  }

  async submitLogbook(studentId: string, batchId: string, payload: any) {
    // Validate word count (min 200 words total)
    const totalWords = (payload.q1_experience + ' ' + payload.q2_progress + ' ' + payload.q3_challenges + ' ' + payload.q4_competencies)
      .trim().split(/\s+/).length;
      
    if (totalWords < 200) {
      throw new BadRequestException('Logbook harus berisi minimal 200 kata secara keseluruhan.');
    }

    let logbook = await this.logbookRepository.findOne({
      where: { studentId, batchId, monthIndex: payload.monthIndex }
    });

    if (logbook && logbook.status === LogbookStatus.ACCEPTED) {
      throw new ForbiddenException('Logbook yang sudah diterima tidak dapat diubah lagi.');
    }

    if (!logbook) {
      logbook = this.logbookRepository.create({
        studentId,
        batchId,
        monthIndex: payload.monthIndex,
      });
    }

    logbook.q1_experience = payload.q1_experience;
    logbook.q2_progress = payload.q2_progress;
    logbook.q3_challenges = payload.q3_challenges;
    logbook.q4_competencies = payload.q4_competencies;
    logbook.status = LogbookStatus.PENDING; // Always reset to pending on submit/resubmit

    return this.logbookRepository.save(logbook);
  }

  async getMentorStudentLogbooks(mentorId: string, batchId: string) {
    // 1. Get all classes mentored by this mentor in this batch
    const mentoredClasses = await this.classRepository.find({
      where: { mentorId, batchId },
      relations: { program: true }
    });

    if (!mentoredClasses.length) return [];

    const classIds = mentoredClasses.map(c => c.id);

    // 2. Get all students enrolled in these classes
    const enrollments = await this.enrollmentRepository.find({
      where: { classId: In(classIds) },
      relations: { student: true }
    });

    if (!enrollments.length) return [];
    
    // Ensure uniqueness if a student is in multiple classes mentored by same mentor (unlikely but safe)
    const uniqueStudentIds = Array.from(new Set(enrollments.map(e => e.studentId)));

    // 3. Get all logbooks for these students in this batch
    const logbooks = await this.logbookRepository.find({
      where: { batchId, studentId: In(uniqueStudentIds) },
      relations: { student: true },
      order: { monthIndex: 'ASC' }
    });

    // We also need the batch total months to render UI properly per student
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    let totalMonths = 0;
    if (batch?.startDate && batch?.endDate) {
      const start = new Date(batch.startDate);
      const end = new Date(batch.endDate);
      totalMonths = (end.getFullYear() - start.getFullYear()) * 12 + (end.getMonth() - start.getMonth()) + 1;
    }
    if (totalMonths <= 0) totalMonths = 1;

    // 4. Group by student
    const studentsMap = new Map();
    for (const enrollment of enrollments) {
      if (!studentsMap.has(enrollment.studentId)) {
        studentsMap.set(enrollment.studentId, {
          student: enrollment.student,
          class: mentoredClasses.find(c => c.id === enrollment.classId),
          logbooks: []
        });
      }
    }

    for (const logbook of logbooks) {
      if (studentsMap.has(logbook.studentId)) {
        studentsMap.get(logbook.studentId).logbooks.push(logbook);
      }
    }

    return {
      totalMonths,
      students: Array.from(studentsMap.values())
    };
  }

  async reviewLogbook(mentorId: string, logbookId: string, status: LogbookStatus, feedback?: string) {
    const logbook = await this.logbookRepository.findOne({ 
      where: { id: logbookId },
      relations: { batch: true }
    });
    
    if (!logbook) throw new NotFoundException('Logbook not found');

    // Verify if mentor has rights to this student's logbook
    const mentoredClasses = await this.classRepository.find({
      where: { mentorId, batchId: logbook.batchId }
    });
    const classIds = mentoredClasses.map(c => c.id);
    const enrollment = await this.enrollmentRepository.findOne({
      where: { studentId: logbook.studentId, classId: In(classIds) }
    });

    if (!enrollment) {
      throw new ForbiddenException('You are not the mentor of this student in this batch');
    }

    logbook.status = status;
    if (feedback !== undefined) {
      logbook.mentorFeedback = feedback;
    }

    return this.logbookRepository.save(logbook);
  }
}
