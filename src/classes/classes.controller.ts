import { Controller, Get, Post, Patch, Delete, Body, UseGuards, Req } from '@nestjs/common';
import { ClassesService } from './classes.service';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard';

@Controller('classes')
export class ClassesController {
  constructor(private readonly classesService: ClassesService) {}

  @UseGuards(SessionAuthGuard)
  @Get('my-classes')
  async getMyClasses(@Req() req: any) {
    return this.classesService.findMyClasses(req.user.id);
  }

  @UseGuards(SessionAuthGuard)
  @Get('mentor-classes')
  async getMentorClasses(@Req() req: any) {
    return this.classesService.findMentorClasses(req.user.id);
  }

  @UseGuards(SessionAuthGuard)
  @Get('programs-list')
  async getProgramsList() {
    return this.classesService.getProgramsWithDetails();
  }

  @UseGuards(SessionAuthGuard)
  @Get('batches')
  async getAllBatches() {
    return this.classesService.getAllBatches();
  }

  @UseGuards(SessionAuthGuard)
  @Post('batches')
  async createGlobalBatch(@Body() body: { name: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[] }) {
    return this.classesService.createGlobalBatch(body);
  }

  @UseGuards(SessionAuthGuard)
  @Patch('batches/:batchId')
  async updateGlobalBatch(@Req() req: any, @Body() body: { name?: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[] }) {
    return this.classesService.updateGlobalBatch(req.params.batchId, body);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('batches/:batchId')
  async deleteGlobalBatch(@Req() req: any) {
    return this.classesService.deleteGlobalBatch(req.params.batchId);
  }

  @UseGuards(SessionAuthGuard)
  @Post('batches/:batchId/import-enroll')
  async importAndEnrollBatch(@Req() req: any, @Body() body: { users: Array<{ name: string; email: string; whatsapp?: string; institution?: string; studyProgram?: string; selectedProgram: string }>; autoDistribute?: boolean }) {
    return this.classesService.importAndEnrollBatch(req.params.batchId, body);
  }

  @UseGuards(SessionAuthGuard)
  @Post('batches/:batchId/assign-mentors')
  async assignBatchMentors(@Req() req: any, @Body() body: { programId: string; mentorIds: string[] }) {
    return this.classesService.assignBatchMentors(req.params.batchId, body.programId, body.mentorIds);
  }

  @UseGuards(SessionAuthGuard)
  @Patch('batch-status')
  async updateBatchStatus(@Body() body: { status: 'active' | 'completed'; batchId?: string; programId?: string }) {
    return this.classesService.updateBatchStatus(body);
  }

  @UseGuards(SessionAuthGuard)
  @Post('program-create-batch')
  async createProgramBatch(@Body() body: { programId: string; batchName: string }) {
    return this.classesService.createProgramBatch(body);
  }

  @UseGuards(SessionAuthGuard)
  @Post('program-enroll')
  async enrollStudent(@Body() body: {
    studentId: string;
    programName: string;
    mentorId?: string;
    isCase3Transfer?: boolean;
  }) {
    return this.classesService.enrollStudentToProgram(body);
  }

  @UseGuards(SessionAuthGuard)
  @Post('program-assign-mentor')
  async assignMentor(@Body() body: { mentorId: string; programName: string }) {
    return this.classesService.assignMentorToProgram(body.mentorId, body.programName);
  }

  @UseGuards(SessionAuthGuard)
  @Post('program-distribute-modulo')
  async distributeModulo(@Body() body: { programName: string }) {
    return this.classesService.distributeModulo(body.programName);
  }

  @UseGuards(SessionAuthGuard)
  @Get('competencies')
  async getCompetencies(@Req() req: any) {
    return this.classesService.getCompetencies(req.query?.programId);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId')
  async getClassDetails(@Req() req: any) {
    return this.classesService.getClassDetails(req.params.classId);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId/material/:materialId')
  async getMaterialDetails(@Req() req: any) {
    return this.classesService.getMaterialDetails(req.params.classId, req.params.materialId);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId/assignment/:assignmentId')
  async getAssignmentDetails(@Req() req: any) {
    return this.classesService.getAssignmentDetails(req.params.classId, req.params.assignmentId);
  }
}

