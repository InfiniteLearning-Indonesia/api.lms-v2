import { Injectable, NotFoundException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity.js';
import { Material } from './entities/material.entity.js';
import { Assignment } from './entities/assignment.entity.js';

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
  ) {}

  async findMyClasses(studentId: string) {
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

    return enrollments.map(e => e.class);
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
          materials: true,
          assignments: true
        },
      });
    }

    // If it doesn't exist in DB (because we haven't seeded), we mock the basic structure
    // so the frontend can still display the beautiful UI.
    if (!classEntity) {
      classEntity = {
        id: classId,
        programId: 'mock-program',
        batchId: 'mock-batch',
        mentorId: 'mock-mentor',
        createdAt: new Date(),
        updatedAt: new Date(),
        program: { id: 'p1', name: '[Gagal mengambil Program dari Database]', description: '', createdAt: new Date(), updatedAt: new Date() },
        batch: { id: 'b1', name: '[Gagal mengambil Batch dari Database]', status: 'active', createdAt: new Date(), updatedAt: new Date() },
        mentor: { name: '[Gagal mengambil Mentor dari Database]' } as any,
        materials: [],
        assignments: [],
      } as Class;
    }

    // Mock Materials and Assignments if empty
    if (!classEntity.materials || classEntity.materials.length === 0) {
      classEntity.materials = [
        {
          id: 'm1', classId, title: '[Gagal mengambil Materi dari Database]', type: 'pdf', url: '#', content: "", createdAt: new Date(), updatedAt: new Date()
        } as Material,
      ];
    }

    if (!classEntity.assignments || classEntity.assignments.length === 0) {
      const futureDate = new Date();
      futureDate.setDate(futureDate.getDate() + 7);
      
      classEntity.assignments = [
        {
          id: 'a1', classId, title: '[Gagal mengambil Tugas dari Database]', description: 'Gagal', dueDate: futureDate, createdAt: new Date(), updatedAt: new Date()
        } as Assignment
      ];
    }

    return classEntity;
  }

  async getMaterialDetails(classId: string, materialId: string) {
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    let materialEntity: Material | null = null;
    
    if (uuidRegex.test(materialId)) {
      materialEntity = await this.materialRepository.findOne({
        where: { id: materialId, classId },
      });
    }

    if (!materialEntity) {
      materialEntity = {
        id: materialId,
        classId,
        title: '[Gagal mengambil Materi dari Database]',
        type: 'pdf',
        url: '#',
        content: 'Data materi tidak ditemukan di database.',
        createdAt: new Date(),
        updatedAt: new Date()
      } as Material;
    }

    return materialEntity;
  }

  async getAssignmentDetails(classId: string, assignmentId: string) {
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    let assignmentEntity: Assignment | null = null;
    
    if (uuidRegex.test(assignmentId)) {
      assignmentEntity = await this.assignmentRepository.findOne({
        where: { id: assignmentId, classId },
      });
    }

    if (!assignmentEntity) {
      const futureDate = new Date();
      futureDate.setDate(futureDate.getDate() + 7);
      assignmentEntity = {
        id: assignmentId,
        classId,
        title: '[Gagal mengambil Tugas dari Database]',
        description: 'Data tugas tidak ditemukan di database.',
        dueDate: futureDate,
        createdAt: new Date(),
        updatedAt: new Date()
      } as Assignment;
    }

    return assignmentEntity;
  }
}
