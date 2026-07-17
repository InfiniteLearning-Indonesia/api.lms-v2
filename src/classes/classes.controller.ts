import { Controller, Get, Post, Put, Patch, Delete, Body, UseGuards, Req, Param } from '@nestjs/common';
import { ClassesService } from './classes.service';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard';
import { RolesGuard } from '../auth/guards/roles.guard';
import { Roles } from '../auth/decorators/roles.decorator';

@Controller('classes')
@UseGuards(SessionAuthGuard, RolesGuard)
export class ClassesController {
  constructor(private readonly classesService: ClassesService) {}

  @Get('my-classes')
  async getMyClasses(@Req() req: any) {
    return this.classesService.findMyClasses(req.user.id);
  }

  @Get('mentor-classes')
  async getMentorClasses(@Req() req: any) {
    return this.classesService.findMentorClasses(req.user.id);
  }

  @Get('programs-list')
  async getProgramsList() {
    return this.classesService.getProgramsWithDetails();
  }

  @Get('batches')
  async getAllBatches() {
    return this.classesService.getAllBatches();
  }

  @Post('batches')
  @Roles('admin')
  async createGlobalBatch(@Body() body: { name: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[] }) {
    return this.classesService.createGlobalBatch(body);
  }

  @Patch('batches/:batchId')
  @Roles('admin')
  async updateGlobalBatch(@Req() req: any, @Body() body: { name?: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[] }) {
    return this.classesService.updateGlobalBatch(req.params.batchId, body);
  }

  @Delete('batches/:batchId')
  @Roles('admin')
  async deleteGlobalBatch(@Req() req: any) {
    return this.classesService.deleteGlobalBatch(req.params.batchId);
  }

  @Post('batches/:batchId/import-enroll')
  @Roles('admin')
  async importAndEnrollBatch(@Req() req: any, @Body() body: { users: Array<{ name: string; email: string; whatsapp?: string; institution?: string; studyProgram?: string; selectedProgram: string }>; autoDistribute?: boolean }) {
    return this.classesService.importAndEnrollBatch(req.params.batchId, body);
  }

  @Post('batches/:batchId/assign-mentors')
  @Roles('admin')
  async assignBatchMentors(@Req() req: any, @Body() body: { programId: string; mentorIds: string[] }) {
    return this.classesService.assignBatchMentors(req.params.batchId, body.programId, body.mentorIds);
  }

  @Patch('batch-status')
  @Roles('admin')
  async updateBatchStatus(@Body() body: { status: 'active' | 'completed'; batchId?: string; programId?: string }) {
    return this.classesService.updateBatchStatus(body);
  }

  @Post('program-create-batch')
  @Roles('admin')
  async createProgramBatch(@Body() body: { programId: string; batchName: string }) {
    return this.classesService.createProgramBatch(body);
  }

  @Post('program-enroll')
  @Roles('admin', 'mentor')
  async enrollStudent(@Body() body: {
    studentId: string;
    programName: string;
    mentorId?: string;
    isCase3Transfer?: boolean;
  }) {
    return this.classesService.enrollStudentToProgram(body);
  }

  @Post('program-assign-mentor')
  @Roles('admin')
  async assignMentor(@Body() body: { mentorId: string; programName: string }) {
    return this.classesService.assignMentorToProgram(body.mentorId, body.programName);
  }

  @Post('program-distribute-modulo')
  @Roles('admin')
  async distributeModulo(@Body() body: { programName: string }) {
    return this.classesService.distributeModulo(body.programName);
  }

  @UseGuards(SessionAuthGuard)
  @Get('competencies')
  async getCompetencies(@Req() req: any) {
    return this.classesService.getCompetencies(req.query?.programId);
  }

  @UseGuards(SessionAuthGuard)
  @Post('competencies')
  async createCompetency(@Req() req: any, @Body() body: { name: string; category: string; programId: string; phase?: string }) {
    return this.classesService.createCompetency(req.user.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Put('competencies/:id')
  async updateCompetency(@Req() req: any, @Body() body: { name?: string; category?: string; phase?: string }) {
    return this.classesService.updateCompetency(req.user.id, req.params.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('competencies/:id')
  async deleteCompetency(@Req() req: any) {
    return this.classesService.deleteCompetency(req.user.id, req.params.id);
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

  @Post('handover-mentor')
  @Roles('admin')
  async handoverMentor(@Body() body: { oldMentorId: string; newMentorId: string; programId: string }) {
    return this.classesService.handoverMentor(body.oldMentorId, body.newMentorId, body.programId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Put('competency/:competencyId/rubric')
  async updateCompetencyRubric(
      @Req() req: any,
      @Param('competencyId') competencyId: string,
      @Body() body: { rubric: any }
  ) {
      return this.classesService.updateCompetencyRubric(competencyId, body.rubric);
  }

  @UseGuards(SessionAuthGuard)
  @Post(':classId/assignment/:assignmentId/submit')
  async submitAssignment(
    @Req() req: any,
    @Param('assignmentId') assignmentId: string,
    @Body() body: { link: string }
  ) {
    return this.classesService.submitAssignment(req.user.id, assignmentId, body.link);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId/assignment/:assignmentId/submissions/me')
  async getMySubmission(
    @Req() req: any,
    @Param('assignmentId') assignmentId: string
  ) {
    return this.classesService.getStudentSubmission(req.user.id, assignmentId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Get(':classId/assignment/:assignmentId/submissions')
  async getSubmissions(@Param('assignmentId') assignmentId: string) {
    return this.classesService.getSubmissions(assignmentId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Put(':classId/assignment/:assignmentId/submissions/:submissionId/grade')
  async gradeSubmission(
    @Req() req: any,
    @Param('submissionId') submissionId: string,
    @Body() body: { score: number; manualFeedback: string }
  ) {
    return this.classesService.gradeSubmission(submissionId, req.user.id, body.score, body.manualFeedback);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post(':classId/assignment/:assignmentId/submissions/:submissionId/ai-evaluate')
  async aiEvaluateSubmission(@Param('submissionId') submissionId: string) {
    return this.classesService.aiEvaluateSubmission(submissionId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Put('assignments/weights')
  async updateAssignmentWeights(@Body() body: { updates: Array<{ id: string; weight: number }> }) {
    return this.classesService.updateAssignmentWeights(body.updates);
  }
}
