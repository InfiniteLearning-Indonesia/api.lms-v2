import { Injectable, Logger, NotFoundException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository, Between, In } from 'typeorm';
import { Attendance, AttendanceStatus } from './entities/attendance.entity';
import { Holiday } from './entities/holiday.entity';
import { Batch } from '../classes/entities/batch.entity';
import { User, UserStatus } from '../users/entities/user.entity';
import { CreateAttendanceDto, BulkCreateAttendanceDto } from './dto/attendance.dto';
import { MentorAsyncDay } from '../classes/entities/mentor-async-day.entity';
import * as crypto from 'crypto';

import { PermissionRequest } from './entities/permission-request.entity';
import { CreatePermissionRequestDto } from './dto/permission-request.dto';

import { StorageService } from '../storage/storage.service';

@Injectable()
export class AttendanceService {
  private readonly logger = new Logger(AttendanceService.name);

  constructor(
    @InjectRepository(Attendance)
    private attendanceRepository: Repository<Attendance>,
    @InjectRepository(Holiday)
    private holidayRepository: Repository<Holiday>,
    @InjectRepository(PermissionRequest)
    private permissionRequestRepository: Repository<PermissionRequest>,
    @InjectRepository(Batch)
    private batchRepository: Repository<Batch>,
    @InjectRepository(User)
    private userRepository: Repository<User>,
    @InjectRepository(MentorAsyncDay)
    private mentorAsyncDayRepository: Repository<MentorAsyncDay>,
    private storageService: StorageService,
  ) {}

  async syncHolidays(year: number): Promise<void> {
    const apiKey = process.env.INDO_API_KEY;
    if (!apiKey) {
      this.logger.warn('INDO_API_KEY is not set in .env. Skipping holiday sync.');
      return;
    }

    // Check if we already have holidays for this year
    const startOfYear = new Date(year, 0, 1);
    const endOfYear = new Date(year, 11, 31, 23, 59, 59);
    
    const count = await this.holidayRepository.count({
      where: { date: Between(startOfYear, endOfYear) }
    });

    if (count > 0) {
      this.logger.log(`Holidays for ${year} already synced (${count} records).`);
      return;
    }

    try {
      this.logger.log(`Fetching holidays for ${year} from apiindonesia.id...`);
      const response = await fetch(`https://use.apiindonesia.id/api/v1/libur?tahun=${year}`, {
        headers: { 'x-api-key': apiKey }
      });
      
      if (!response.ok) {
        throw new Error(`Failed to fetch holidays: ${response.statusText}`);
      }

      const data = await response.json();
      if (data && data.data && Array.isArray(data.data)) {
        for (const item of data.data) {
          if (item.is_active === 1) {
            const h = new Holiday();
            h.date = new Date(item.date);
            h.name = item.name;
            h.type = item.type;
            h.isActive = item.is_active;
            await this.holidayRepository.save(h);
          }
        }
        this.logger.log(`Successfully synced holidays for ${year}`);
      }
    } catch (error) {
      this.logger.error(`Error syncing holidays: ${error.message}`);
    }
  }

  async getHolidays(year: number): Promise<Holiday[]> {
    await this.syncHolidays(year);
    const startOfYear = new Date(year, 0, 1);
    const endOfYear = new Date(year, 11, 31, 23, 59, 59);
    return this.holidayRepository.find({
      where: { date: Between(startOfYear, endOfYear) }
    });
  }

  async getBatchActiveDays(batchId: string, month?: number, year?: number): Promise<{ total: number, days: Date[], holidays: { date: string, name: string }[] }> {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch || !batch.startDate || !batch.endDate) {
      return { total: 0, days: [], holidays: [] };
    }

    const start = new Date(batch.startDate);
    const end = new Date(batch.endDate);
    
    let queryStart = start;
    let queryEnd = end;

    if (month !== undefined && year !== undefined) {
      const monthStart = new Date(year, month - 1, 1);
      const monthEnd = new Date(year, month, 0, 23, 59, 59);
      queryStart = start > monthStart ? start : monthStart;
      queryEnd = end < monthEnd ? end : monthEnd;
    }

    // Ensure year parameter triggers holiday sync if needed
    const y = year || queryStart.getFullYear();
    const holidays = await this.getHolidays(y);
    const holidayDates = holidays.map(h => {
      const d = new Date(h.date);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    });

    const activeDays: Date[] = [];
    let current = new Date(queryStart);
    current.setHours(0, 0, 0, 0);
    const endLimit = new Date(queryEnd);
    endLimit.setHours(23, 59, 59, 999);

    while (current <= endLimit) {
      const dayOfWeek = current.getDay();
      // Skip weekends (0 = Sunday, 6 = Saturday) and Fridays (5 = Asynchronous Off-day)
      if (dayOfWeek !== 0 && dayOfWeek !== 6 && dayOfWeek !== 5) {
        // Format to YYYY-MM-DD using local timezone to prevent UTC shift
        const dateString = `${current.getFullYear()}-${String(current.getMonth() + 1).padStart(2, '0')}-${String(current.getDate()).padStart(2, '0')}`;
        // Skip national holidays
        if (!holidayDates.includes(dateString)) {
          activeDays.push(new Date(current));
        }
      }
      current.setDate(current.getDate() + 1);
    }

    const holidayList = holidays.map(h => {
      const d = new Date(h.date);
      return {
        date: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`,
        name: h.name
      };
    });

    return { total: activeDays.length, days: activeDays, holidays: holidayList };
  }

  async updateSpLogic(studentId: string, batchId: string, date: Date) {
    const month = date.getMonth() + 1;
    const year = date.getFullYear();

    // Calculate total active days in this month for the batch
    const activeDaysInfo = await this.getBatchActiveDays(batchId, month, year);
    const totalActiveDays = activeDaysInfo.total;

    if (totalActiveDays === 0) return;

    // Get all attendances for this student in this month
    const startOfMonth = new Date(year, month - 1, 1);
    const endOfMonth = new Date(year, month, 0, 23, 59, 59);

    const attendances = await this.attendanceRepository.find({
      where: {
        studentId,
        batchId,
        date: Between(startOfMonth, endOfMonth)
      }
    });

    const alphas = attendances.filter(a => a.status === AttendanceStatus.ALPHA).length;
    const threshold = Math.max(1, Math.floor(totalActiveDays * 0.10)); // 10%

    const student = await this.userRepository.findOne({ where: { id: studentId } });
    if (!student) return;

    let newSpLevel = 0;
    
    if (alphas >= threshold) {
      const excess = alphas - threshold;
      if (excess === 0) newSpLevel = 1;      // SP1 when hitting 10% active days
      else if (excess === 1) newSpLevel = 2; // SP2 when 1 more alpha after SP1
      else if (excess === 2) newSpLevel = 3; // SP3 when 1 more alpha after SP2
      else if (excess >= 3) {
        newSpLevel = 4; // Suspended when 1 more alpha after SP3
        if (student.status !== UserStatus.SUSPENDED) {
          student.status = UserStatus.SUSPENDED;
          await this.userRepository.save(student);
          this.logger.log(`Student ${student.email} automatically suspended due to SP4 limit.`);
        }
      }
    }

    // Update SP Level on all attendances of this student in this month
    for (const a of attendances) {
      if (a.spLevel !== newSpLevel) {
        a.spLevel = newSpLevel;
        await this.attendanceRepository.save(a);
      }
    }
  }

  async upsertAttendance(dto: CreateAttendanceDto) {
    const targetDate = new Date(dto.date);
    targetDate.setHours(12, 0, 0, 0); // Normalize to midday to avoid timezone shifts

    // Check if student is suspended (suspended accounts automatically remain Alpha)
    const student = await this.userRepository.findOne({ where: { id: dto.studentId } });
    let finalStatus = dto.status;
    if (student && student.status === UserStatus.SUSPENDED) {
      finalStatus = AttendanceStatus.ALPHA;
    } else {
      // Check if student has submitted a Form Izin (PermissionRequest) for this date
      const dateStr = dto.date.split('T')[0];
      const permReqs = await this.permissionRequestRepository.find({
        where: { studentId: dto.studentId, batchId: dto.batchId }
      });
      const hasPermReq = permReqs.some(p => p.date && p.date.split('T')[0] === dateStr);
      if (hasPermReq) {
        finalStatus = AttendanceStatus.IZIN_SAKIT;
      }
    }

    let attendance = await this.attendanceRepository.findOne({
      where: {
        studentId: dto.studentId,
        batchId: dto.batchId,
        date: targetDate
      }
    });

    if (attendance) {
      attendance.status = finalStatus;
    } else {
      attendance = this.attendanceRepository.create({
        studentId: dto.studentId,
        batchId: dto.batchId,
        date: targetDate,
        status: finalStatus,
        spLevel: 0
      });
    }

    await this.attendanceRepository.save(attendance);
    
    // Evaluate SP Logic
    await this.updateSpLogic(dto.studentId, dto.batchId, targetDate);

    return attendance;
  }

  async bulkUpsertAttendance(dto: BulkCreateAttendanceDto) {
    const results: Attendance[] = [];
    for (const record of dto.attendances) {
      const res = await this.upsertAttendance(record);
      results.push(res);
    }
    return results;
  }

  async getAttendances(batchId?: string, studentId?: string, mentorId?: string) {
    const query = this.attendanceRepository.createQueryBuilder('attendance')
      .leftJoinAndSelect('attendance.student', 'student')
      .leftJoinAndSelect('attendance.batch', 'batch');

    if (batchId) {
      query.andWhere('attendance.batchId = :batchId', { batchId });
    }

    if (studentId) {
      query.andWhere('attendance.studentId = :studentId', { studentId });
    }

    // If mentorId is provided, we need to filter students that are assigned to this mentor.
    // Assuming Enrollment entity holds this relationship, but for simplicity we might need to join it.
    // Let's implement mentor filter via subquery if needed, or join enrollments.
    if (mentorId) {
       query.leftJoin('enrollments', 'en', 'en."studentId" = attendance."studentId"')
            .leftJoin('classes', 'cls', 'cls.id = en."classId"')
            .andWhere('cls."batchId" = attendance."batchId"')
            .andWhere('cls."mentorId" = :mentorId', { mentorId });
    }

    query.orderBy('attendance.date', 'DESC');
    return query.getMany();
  }

  async getMyStudentsAttendance(mentorId: string, batchId: string) {
    return this.getAttendances(batchId, undefined, mentorId);
  }

  async unsuspendStudent(studentId: string) {
    const student = await this.userRepository.findOne({ where: { id: studentId } });
    if (!student) throw new NotFoundException('Student not found');
    
    if (student.status === UserStatus.SUSPENDED) {
      student.status = UserStatus.ACTIVE;
      await this.userRepository.save(student);
      
      // Reset SP level for the current month so they don't immediately get suspended again
      const now = new Date();
      const startOfMonth = new Date(now.getFullYear(), now.getMonth(), 1);
      const endOfMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0, 23, 59, 59);

      const attendances = await this.attendanceRepository.find({
        where: {
          studentId,
          date: Between(startOfMonth, endOfMonth)
        }
      });
      
      // Let's reset SP to 0 for them
      for (const a of attendances) {
        a.spLevel = 0;
        await this.attendanceRepository.save(a);
      }
      
      return { success: true, message: 'Student unsuspended' };
    }
  }

  async getActiveDaysForDateRange(batchId: string, startDate?: Date | null, endDate?: Date | null): Promise<{ total: number, days: Date[], activeDateStrings: string[] }> {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    const start = startDate ? new Date(startDate) : (batch?.startDate ? new Date(batch.startDate) : null);
    const end = endDate ? new Date(endDate) : (batch?.endDate ? new Date(batch.endDate) : null);

    if (!start || !end) {
      return { total: 0, days: [], activeDateStrings: [] };
    }

    const y = start.getFullYear();
    const holidays = await this.getHolidays(y);
    const holidayDates = holidays.map(h => {
      const d = new Date(h.date);
      return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    });

    const mentorAsyncDays = await this.mentorAsyncDayRepository.find();
    const asyncDateStrings = mentorAsyncDays.map(a => a.date);

    const activeDays: Date[] = [];
    const activeDateStrings: string[] = [];

    let current = new Date(start);
    current.setHours(0, 0, 0, 0);
    const endLimit = new Date(end);
    endLimit.setHours(23, 59, 59, 999);

    while (current <= endLimit) {
      const dayOfWeek = current.getDay();
      // Skip weekends (0 = Sunday, 6 = Saturday) and Fridays (5 = Asynchronous Off-day)
      if (dayOfWeek !== 0 && dayOfWeek !== 6 && dayOfWeek !== 5) {
        const dateString = `${current.getFullYear()}-${String(current.getMonth() + 1).padStart(2, '0')}-${String(current.getDate()).padStart(2, '0')}`;
        // Skip national holidays & mentor async days
        if (!holidayDates.includes(dateString) && !asyncDateStrings.includes(dateString)) {
          activeDays.push(new Date(current));
          activeDateStrings.push(dateString);
        }
      }
      current.setDate(current.getDate() + 1);
    }

    return { total: activeDays.length, days: activeDays, activeDateStrings };
  }

  async calculateStudentPhaseScore(batchId: string, studentId: string, startDate?: Date | null, endDate?: Date | null): Promise<{
    totalSyncDays: number;
    alphaDays: number;
    cleanAttendance: number;
    score: number;
  }> {
    const activeInfo = await this.getActiveDaysForDateRange(batchId, startDate, endDate);
    const totalSyncDays = activeInfo.total;

    if (totalSyncDays === 0) {
      return { totalSyncDays: 0, alphaDays: 0, cleanAttendance: 0, score: 65.0 };
    }

    const start = startDate ? new Date(startDate) : new Date(0);
    const end = endDate ? new Date(endDate) : new Date(2099, 11, 31);
    start.setHours(0, 0, 0, 0);
    end.setHours(23, 59, 59, 999);

    const attendances = await this.attendanceRepository.find({
      where: {
        studentId,
        batchId,
        date: Between(start, end)
      }
    });

    const alphaDays = attendances.filter(a => {
      if (a.status !== AttendanceStatus.ALPHA) return false;
      const d = new Date(a.date);
      const ds = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
      return activeInfo.activeDateStrings.includes(ds);
    }).length;

    const cleanAttendance = Math.max(0, totalSyncDays - alphaDays);
    const rawScore = (cleanAttendance / totalSyncDays) * 95;
    const score = Math.max(65.0, Math.min(95.0, Math.round(rawScore * 10) / 10));

    return {
      totalSyncDays,
      alphaDays,
      cleanAttendance,
      score
    };
  }

  async getBatchPhaseAttendanceScores(batchId: string) {
    const batch = await this.batchRepository.findOne({ where: { id: batchId } });
    if (!batch) return {};

    let microStart = batch.microStartDate || batch.startDate;
    let microEnd = batch.microEndDate;
    let massiveStart = batch.massiveStartDate;
    let massiveEnd = batch.massiveEndDate || batch.endDate;

    // If microEndDate / massiveStartDate not explicitly set, calculate midpoint
    if (!microEnd && batch.startDate && batch.endDate) {
      const startTime = new Date(batch.startDate).getTime();
      const endTime = new Date(batch.endDate).getTime();
      const midTime = startTime + (endTime - startTime) / 2;
      microEnd = new Date(midTime);
      massiveStart = new Date(midTime + 86400000);
    }

    // Get all attendances in this batch
    const attendances = await this.attendanceRepository.find({ where: { batchId } });
    const studentIds = Array.from(new Set(attendances.map(a => a.studentId)));

    const scoresMap: Record<string, any> = {};

    for (const studentId of studentIds) {
      const microRes = await this.calculateStudentPhaseScore(batchId, studentId, microStart, microEnd);
      const massiveRes = await this.calculateStudentPhaseScore(batchId, studentId, massiveStart, massiveEnd);

      scoresMap[studentId] = {
        microScore: microRes.score,
        massiveScore: massiveRes.score,
        microDetails: microRes,
        massiveDetails: massiveRes,
      };
    }

    return scoresMap;
  }

  async createPermissionRequest(dto: CreatePermissionRequestDto) {
    // Upload files to Cloudflare R2
    const uploadedProofFiles = await this.storageService.uploadMultipleBase64(
      dto.proofFiles || [],
      'permissions/proofs',
    );
    const uploadedMentorChatFiles = await this.storageService.uploadMultipleBase64(
      dto.mentorChatFiles || [],
      'permissions/chat-proofs',
    );

    const permReq = this.permissionRequestRepository.create({
      studentId: dto.studentId,
      batchId: dto.batchId,
      date: dto.date,
      category: dto.category,
      reason: dto.reason,
      proofFiles: uploadedProofFiles,
      mentorChatFiles: uploadedMentorChatFiles,
    });

    const saved = await this.permissionRequestRepository.save(permReq);

    // Automatically upsert attendance as Izin/Sakit for that date
    await this.upsertAttendance({
      studentId: dto.studentId,
      batchId: dto.batchId,
      date: dto.date,
      status: AttendanceStatus.IZIN_SAKIT,
    });

    return saved;
  }

  async getPermissionRequests(batchId?: string, studentId?: string, date?: string, mentorId?: string) {
    const query = this.permissionRequestRepository.createQueryBuilder('perm')
      .leftJoinAndSelect('perm.student', 'student')
      .leftJoinAndSelect('perm.batch', 'batch');

    if (batchId) {
      query.andWhere('perm.batchId = :batchId', { batchId });
    }

    if (studentId) {
      query.andWhere('perm.studentId = :studentId', { studentId });
    }

    if (date) {
      query.andWhere('perm.date = :date', { date });
    }

    if (mentorId) {
      query.leftJoin('enrollments', 'en', 'en."studentId" = perm."studentId"')
           .leftJoin('classes', 'cls', 'cls.id = en."classId"')
           .andWhere('cls."batchId" = perm."batchId"')
           .andWhere('cls."mentorId" = :mentorId', { mentorId });
    }

    query.orderBy('perm.createdAt', 'DESC');
    return query.getMany();
  }
}
