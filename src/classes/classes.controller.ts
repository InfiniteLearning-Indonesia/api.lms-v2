import { Controller, Get, UseGuards, Req } from '@nestjs/common';
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
