import {
  Controller,
  Get,
  Post,
  Body,
  Param,
  UseGuards,
  Req,
  Query,
} from '@nestjs/common';
import { AttendanceService } from './attendance.service';
import {
  CreateAttendanceDto,
  BulkCreateAttendanceDto,
} from './dto/attendance.dto';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard.js';
import { RolesGuard } from '../auth/guards/roles.guard.js';
import { Roles } from '../auth/decorators/roles.decorator.js';

import { CreatePermissionRequestDto } from './dto/permission-request.dto';

@Controller('attendance')
@UseGuards(SessionAuthGuard, RolesGuard)
export class AttendanceController {
  constructor(private readonly attendanceService: AttendanceService) {}

  @Get('holidays/:year')
  async getHolidays(@Param('year') year: string) {
    return this.attendanceService.getHolidays(parseInt(year));
  }

  @Get('active-days/:batchId')
  async getActiveDays(
    @Param('batchId') batchId: string,
    @Query('month') month?: string,
    @Query('year') year?: string,
  ) {
    const m = month ? parseInt(month) : undefined;
    const y = year ? parseInt(year) : undefined;
    return this.attendanceService.getBatchActiveDays(batchId, m, y);
  }

  @Post('permission-requests')
  async createPermissionRequest(@Body() dto: CreatePermissionRequestDto) {
    return this.attendanceService.createPermissionRequest(dto);
  }

  @Get('permission-requests')
  async getPermissionRequests(
    @Query('batchId') batchId?: string,
    @Query('studentId') studentId?: string,
    @Query('date') date?: string,
    @Query('mentorId') mentorId?: string,
  ) {
    return this.attendanceService.getPermissionRequests(
      batchId,
      studentId,
      date,
      mentorId,
    );
  }

  @Post()
  @Roles('admin', 'mentor', 'facilitator')
  async upsertAttendance(@Body() dto: CreateAttendanceDto) {
    return this.attendanceService.upsertAttendance(dto);
  }

  @Post('bulk')
  @Roles('admin', 'mentor', 'facilitator')
  async bulkUpsertAttendance(@Body() dto: BulkCreateAttendanceDto) {
    return this.attendanceService.bulkUpsertAttendance(dto);
  }

  @Get()
  async getAllAttendances(
    @Req() req: any,
    @Query('batchId') batchId?: string,
    @Query('studentId') studentId?: string,
    @Query('mentorId') mentorId?: string,
  ) {
    return this.attendanceService.getAttendances(
      req.user,
      batchId,
      studentId,
      mentorId,
    );
  }

  @Get('scores')
  async getPhaseAttendanceScores(@Query('batchId') batchId: string) {
    return this.attendanceService.getBatchPhaseAttendanceScores(batchId);
  }

  @Post('unsuspend/:studentId')
  @Roles('admin')
  async unsuspendStudent(@Param('studentId') studentId: string) {
    return this.attendanceService.unsuspendStudent(studentId);
  }
}
