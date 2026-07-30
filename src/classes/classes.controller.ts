import { Controller, Get, Post, Put, Patch, Delete, Body, UseGuards, Req, Param } from '@nestjs/common';
import { ClassesService } from './classes.service';
import { SessionAuthGuard } from '../auth/guards/session-auth.guard';
import { RolesGuard } from '../auth/guards/roles.guard';
import { Roles } from '../auth/decorators/roles.decorator';

@Controller('classes')
@UseGuards(SessionAuthGuard, RolesGuard)
export class ClassesController {
  constructor(private readonly classesService: ClassesService) { }

  @Get('my-classes')
  async getMyClasses(@Req() req: any) {
    return this.classesService.findMyClasses(req.user.id);
  }

  @Get('my-grades')
  async getMyGrades(@Req() req: any) {
    return this.classesService.findMyGrades(req.user.id);
  }

  @Post('programs/:programId/release-certificate')
  async releaseCertificate(@Param('programId') programId: string, @Req() req: any) {
    return this.classesService.releaseCertificate(req.user.id, programId);
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
  async createGlobalBatch(@Body() body: { name: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[]; startDate?: string; endDate?: string }) {
    return this.classesService.createGlobalBatch(body);
  }

  @Patch('batches/:batchId')
  @Roles('admin')
  async updateGlobalBatch(@Req() req: any, @Body() body: { name?: string; status?: string; includedProgramIds?: string[]; newProgramNames?: string[]; startDate?: string; endDate?: string }) {
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

  @Post('batches/:batchId/mentor-matrix')
  @Roles('admin')
  async saveMentorMatrix(@Param('batchId') batchId: string, @Body() body: { matrix: Record<string, any> }) {
    return this.classesService.saveMentorMatrix(batchId, body.matrix);
  }

  @Post('batches/:batchId/activate')
  @Roles('admin')
  async activateBatch(@Param('batchId') batchId: string) {
    return this.classesService.updateBatchStatus({ status: 'active', batchId });
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

  @Post('programs/:programId/enroll-student')
  @Roles('admin', 'mentor')
  async enrollStudentByProgramId(
    @Param('programId') programId: string,
    @Body() body: { studentId: string; mentorId?: string; cleanTransfer?: boolean; isCase3Transfer?: boolean },
  ) {
    return this.classesService.enrollStudentToProgramById(programId, body);
  }

  @Post('program-assign-mentor')
  @Roles('admin')
  async assignMentor(@Body() body: { mentorId: string; programName: string }) {
    return this.classesService.assignMentorToProgram(body.mentorId, body.programName);
  }

  @Post('programs/:programId/assign-mentor')
  @Roles('admin')
  async assignMentorByProgramId(
    @Param('programId') programId: string,
    @Body() body: { mentorId: string },
  ) {
    return this.classesService.assignMentorToProgramById(programId, body.mentorId);
  }

  @Post('program-distribute-modulo')
  @Roles('admin', 'mentor')
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
  async createCompetency(@Req() req: any, @Body() body: { name: string; category: string; programId: string; programCompetencyId?: string }) {
    return this.classesService.createCompetency(req.user.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Put('competencies/:id')
  async updateCompetency(@Req() req: any, @Body() body: { name?: string; category?: string; programCompetencyId?: string }) {
    return this.classesService.updateCompetency(req.user.id, req.params.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('competencies/:id')
  async deleteCompetency(@Req() req: any) {
    return this.classesService.deleteCompetency(req.user.id, req.params.id);
  }

  // --- Program Competency Endpoints ---

  @UseGuards(SessionAuthGuard)
  @Get('program-competencies')
  async getProgramCompetencies(@Req() req: any) {
    return this.classesService.getProgramCompetencies(req.query?.programId);
  }

  @UseGuards(SessionAuthGuard)
  @Post('program-competencies')
  async createProgramCompetency(@Body() body: { name: string; category: string; programId?: string; isGlobal?: boolean; syllabuses?: { name: string }[] }) {
    return this.classesService.createProgramCompetency(body);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('program-competencies/:id')
  async deleteProgramCompetency(@Req() req: any) {
    return this.classesService.deleteProgramCompetency(req.params.id);
  }


  @UseGuards(SessionAuthGuard)
  @Get('rubrik-assessments')
  async getAllRubrikAssessments(@Req() req: any) {
    return this.classesService.getRubrikAssessmentsByProgram(req.query?.programId);
  }

  @UseGuards(SessionAuthGuard)
  @Get('programs/:programId/rubrik-assessments')
  async getRubrikAssessmentsByProgram(@Req() req: any, @Param('programId') programId: string) {
    return this.classesService.getRubrikAssessmentsByProgram(programId);
  }

  @UseGuards(SessionAuthGuard)
  @Post('rubrik-assessments')
  async createRubrikAssessment(@Req() req: any, @Body() body: { name: string; programId: string; phase?: string; competencies?: any[]; subAssessments?: any[] }) {
    return this.classesService.createRubrikAssessment(req.user.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Put('rubrik-assessments/:id')
  async updateRubrikAssessment(@Req() req: any, @Param('id') id: string, @Body() body: { name?: string; phase?: string; competencies?: any[]; subAssessments?: any[] }) {
    return this.classesService.updateRubrikAssessment(req.user.id, id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('rubrik-assessments/:id')
  async deleteRubrikAssessment(@Req() req: any, @Param('id') id: string) {
    return this.classesService.deleteRubrikAssessment(req.user.id, id);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('materials/:id')
  async deleteMaterial(@Req() req: any, @Param('id') id: string) {
    return this.classesService.deleteMaterial(req.user.id, id);
  }

  @UseGuards(SessionAuthGuard)
  @Delete('assignments/:id')
  async deleteAssignment(@Req() req: any, @Param('id') id: string) {
    return this.classesService.deleteAssignment(req.user.id, id);
  }

  @UseGuards(SessionAuthGuard)
  @Get('programs/:programId/rubrik-assessments/scores')
  async getAssessmentScores(@Param('programId') programId: string) {
    return this.classesService.getAssessmentScores(programId);
  }

  @UseGuards(SessionAuthGuard)
  @Get('programs/:programId/competencies/scores')
  async getProgramCompetencyScores(@Param('programId') programId: string) {
    return this.classesService.getCompetencyScores(programId);
  }

  @UseGuards(SessionAuthGuard)
  @Get('competencies/scores')
  async getGlobalCompetencyScores() {
    return this.classesService.getCompetencyScores();
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post('competencies/scores')
  async upsertCompetencyScore(
    @Req() req: any,
    @Body() body: { studentId: string; competencyId: string; score: number }
  ) {
    return this.classesService.upsertCompetencyScore(body.studentId, body.competencyId, body.score);
  }

  @UseGuards(SessionAuthGuard)
  @Post('programs/:programId/rubrik-assessments/import-scores')
  async importAssessmentScores(
    @Req() req: any,
    @Param('programId') programId: string,
    @Body() body: { scores: Array<{ email?: string, name?: string, rubrikAssessmentId: string, score: number }> }
  ) {
    return this.classesService.importAssessmentScores(req.user.id, programId, body.scores);
  }

  @UseGuards(SessionAuthGuard)
  @Post('programs/:programId/smart-import-scores')
  async smartImportScores(
    @Req() req: any,
    @Param('programId') programId: string,
    @Body() body: {
      newColumns?: Array<{ name: string; category?: string }>;
      scores: Array<{ email?: string; name?: string; targetType?: 'competency' | 'rubrik'; targetId?: string; columnName?: string; score: number }>;
    }
  ) {
    return this.classesService.smartImportScores(req.user.id, programId, body);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId')
  async getClassDetails(@Req() req: any) {
    return this.classesService.getClassDetails(req.params.classId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post(':classId/material')
  async createMaterial(@Req() req: any, @Param('classId') classId: string, @Body() body: any) {
    return this.classesService.createMaterial(req.user.id, classId, body);
  }

  @UseGuards(SessionAuthGuard)
  @Get(':classId/material/:materialId')
  async getMaterialDetails(@Req() req: any) {
    return this.classesService.getMaterialDetails(req.params.classId, req.params.materialId);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post(':classId/assignment')
  async createAssignment(@Req() req: any, @Param('classId') classId: string, @Body() body: any) {
    return this.classesService.createAssignment(req.user.id, classId, body);
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

  @Post('user-batches')
  @Roles('admin')
  async updateUserBatches(@Body() body: { userId: string; batchIds: string[] }) {
    return this.classesService.updateUserBatches(body.userId, body.batchIds);
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
  @Get('mentor/ai-config')
  async getMentorAiConfig(@Req() req: any) {
    return this.classesService.getMentorAiConfig(req.user.id);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post('mentor/ai-config')
  async saveMentorAiConfig(@Req() req: any, @Body() body: any) {
    return this.classesService.saveMentorAiConfig(req.user.id, body);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post('mentor/ai-models')
  async fetchAiModels(@Body() body: { provider: string; hostOrApiKey?: string }) {
    return this.classesService.fetchAiModels(body.provider, body.hostOrApiKey);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Post(':classId/assignment/:assignmentId/bulk-ai-evaluate')
  async bulkAiEvaluateSubmissions(
    @Req() req: any,
    @Param('assignmentId') assignmentId: string,
    @Body() body: {
      submissionIds?: string[];
      batchSize?: number;
      provider?: string;
      model?: string;
      ollamaHost?: string;
      groqApiKey?: string;
      googleAiStudioKey?: string;
    },
  ) {
    return this.classesService.bulkAiEvaluateSubmissions(req.user.id, assignmentId, body);
  }

  @UseGuards(SessionAuthGuard)
  @Roles('mentor', 'admin')
  @Put('assignments/weights')
  async updateAssignmentWeights(@Body() body: { updates: Array<{ id: string; weight: number }> }) {
    return this.classesService.updateAssignmentWeights(body.updates);
  }

  // ================= LOGBOOK BULANAN =================
  
  @Get('batches/:batchId/logbooks/student')
  @Roles('student')
  async getStudentLogbooks(@Req() req: any, @Param('batchId') batchId: string) {
    return this.classesService.getStudentLogbooks(req.user.id, batchId);
  }

  @Post('batches/:batchId/logbooks/student')
  @Roles('student')
  async submitLogbook(@Req() req: any, @Param('batchId') batchId: string, @Body() body: any) {
    return this.classesService.submitLogbook(req.user.id, batchId, body);
  }

  @Get('batches/:batchId/logbooks/mentor')
  @Roles('mentor')
  async getMentorStudentLogbooks(@Req() req: any, @Param('batchId') batchId: string) {
    return this.classesService.getMentorStudentLogbooks(req.user.id, batchId);
  }

  @Patch('batches/logbooks/:logbookId/review')
  @Roles('mentor')
  async reviewLogbook(@Req() req: any, @Param('logbookId') logbookId: string, @Body() body: { status: any, feedback?: string }) {
    return this.classesService.reviewLogbook(req.user.id, logbookId, body.status, body.feedback);
  }

  // ================= HARI ASYNCHRONOUS MENTOR =================

  @Get('attendance/async-days/mentor')
  @Roles('mentor')
  async getMentorAsyncDays(@Req() req: any) {
    return this.classesService.getMentorAsyncDays(req.user.id);
  }

  @Post('attendance/async-days/toggle')
  @Roles('mentor')
  async toggleMentorAsyncDay(@Req() req: any, @Body() body: { date: string; note?: string }) {
    return this.classesService.toggleMentorAsyncDay(req.user.id, body.date, body.note);
  }

  @Get('attendance/async-days/student')
  @Roles('student')
  async getStudentMentorAsyncDays(@Req() req: any) {
    return this.classesService.getStudentMentorAsyncDays(req.user.id);
  }

  @UseGuards(SessionAuthGuard)
  @Put('batches/:batchId/phase-dates')
  async updateBatchPhaseDates(
    @Param('batchId') batchId: string,
    @Body() body: { microStartDate?: string; microEndDate?: string; massiveStartDate?: string; massiveEndDate?: string }
  ) {
    return this.classesService.updateBatchPhaseDates(batchId, body);
  }
}
