import { Controller, Get, Post, Body, Param, UseGuards, Request, Query } from '@nestjs/common';
import { AttendanceService } from './attendance.service';
import { CreateAttendanceDto, BulkCreateAttendanceDto } from './dto/attendance.dto';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard.js';

@Controller('attendance')
export class AttendanceController {
  constructor(private readonly attendanceService: AttendanceService) {}

  @UseGuards(SessionAuthGuard)
  @Get('holidays/:year')
  async getHolidays(@Param('year') year: string) {
    return this.attendanceService.getHolidays(parseInt(year));
  }

  @UseGuards(SessionAuthGuard)
  @Get('active-days/:batchId')
  async getActiveDays(
    @Param('batchId') batchId: string,
    @Query('month') month?: string,
    @Query('year') year?: string
  ) {
    const m = month ? parseInt(month) : undefined;
    const y = year ? parseInt(year) : undefined;
    return this.attendanceService.getBatchActiveDays(batchId, m, y);
  }

  @UseGuards(SessionAuthGuard)
  @Post()
  async upsertAttendance(@Body() dto: CreateAttendanceDto) {
    return this.attendanceService.upsertAttendance(dto);
  }

  @UseGuards(SessionAuthGuard)
  @Post('bulk')
  async bulkUpsertAttendance(@Body() dto: BulkCreateAttendanceDto) {
    return this.attendanceService.bulkUpsertAttendance(dto);
  }

  @UseGuards(SessionAuthGuard)
  @Get()
  async getAllAttendances(
    @Query('batchId') batchId?: string,
    @Query('studentId') studentId?: string,
    @Query('mentorId') mentorId?: string
  ) {
    return this.attendanceService.getAttendances(batchId, studentId, mentorId);
  }

  @UseGuards(SessionAuthGuard)
  @Post('unsuspend/:studentId')
  async unsuspendStudent(@Param('studentId') studentId: string) {
    return this.attendanceService.unsuspendStudent(studentId);
  }
}
