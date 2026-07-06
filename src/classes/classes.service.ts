import { Injectable, NotFoundException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Enrollment } from './entities/enrollment.entity';
import { Class } from './entities/class.entity.js';
import { Material } from './entities/material.entity.js';
import { Assignment } from './entities/assignment.entity.js';
import { Competency } from './entities/competency.entity.js';

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

  async findMentorClasses(mentorId: string) {
    let classes = await this.classRepository.find({
      where: { mentorId },
      relations: {
        program: true,
        batch: true,
        materials: true,
        assignments: true,
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
        return {
          ...cls,
          enrolledStudentsCount: enrolledStudents.length,
          enrolledStudents,
        };
      }),
    );

    if (enrichedClasses.length > 0) {
      return enrichedClasses;
    }

    // Fallback mock for mentor view when DB not seeded or mentor has no assigned classes yet
    return [
      {
        id: '550e8400-e29b-41d4-a716-446655440000',
        programId: 'prog-web',
        batchId: 'batch-7',
        mentorId,
        program: { id: 'prog-web', name: 'Web Development and UI/UX Design', description: 'Program Kolaboratif Web & UI/UX' },
        batch: { id: 'batch-7', name: 'Batch 7 - 2026', status: 'active' },
        materials: [
          { id: 'm1', title: 'Pengenalan Web Semantik & HTML5', type: 'pdf', competency: 'Kompetensi 1: Fundamental Web' },
          { id: 'm2', title: 'Arsitektur CSS Modern (Flexbox & Grid)', type: 'video', competency: 'Kompetensi 1: Fundamental Web' },
        ],
        assignments: [
          { id: 'a1', title: 'Tugas Praktik 1: Layout Responsive', dueDate: new Date(Date.now() + 604800000), competency: 'Kompetensi 1: Fundamental Web' },
        ],
        enrolledStudentsCount: 24,
        enrolledStudents: [
          { id: 's1', name: 'Student Riyanda', email: 'riyandaazis00@gmail.com', whatsapp: '081263666474', selectedProgram: 'Web Development and UI/UX Design', status: 'active' },
          { id: 's2', name: 'Ahmad Siswa Web', email: 'ahmad.siswa@gmail.com', whatsapp: '081234567890', selectedProgram: 'Web Development and UI/UX Design', status: 'active' },
          { id: 's3', name: 'Budi Non Gmail', email: 'budi.non@il.com', whatsapp: '081998877665', selectedProgram: 'Web Development and UI/UX Design', status: 'invited' },
        ],
      },
      {
        id: '550e8400-e29b-41d4-a716-446655440002',
        programId: 'prog-mobile',
        batchId: 'batch-7',
        mentorId,
        program: { id: 'prog-mobile', name: 'Mobile Development and UI/UX Design', description: 'Program Kolaboratif Mobile & UI/UX' },
        batch: { id: 'batch-7', name: 'Batch 7 - 2026', status: 'active' },
        materials: [
          { id: 'm3', title: 'Pengenalan Flutter & Dart', type: 'video', competency: 'Kompetensi 1: Mobile UI Architecture' },
        ],
        assignments: [
          { id: 'a2', title: 'Tugas Praktik Flutter App', dueDate: new Date(Date.now() + 604800000), competency: 'Kompetensi 1: Mobile UI Architecture' },
        ],
        enrolledStudentsCount: 18,
        enrolledStudents: [
          { id: 's4', name: 'Siti Mobile', email: 'siti.mobile@gmail.com', whatsapp: '081333444555', selectedProgram: 'Mobile Development and UI/UX Design', status: 'active' },
          { id: 's5', name: 'Reza Android', email: 'reza.dev@gmail.com', whatsapp: '081222333444', selectedProgram: 'Mobile Development and UI/UX Design', status: 'active' },
        ],
      },
    ];
  }

  async getCompetencies(programId?: string) {
    const whereClause = programId ? { programId } : {};
    const competencies = await this.competencyRepository.find({
      where: whereClause,
      relations: { program: true, creatorMentor: true },
    });

    if (competencies.length > 0) {
      return competencies;
    }

    // Fallback mock competencies from SourceOfTruth.MD
    return [
      {
        id: 'comp-1',
        programId: 'prog-web',
        name: 'Kompetensi 1: Fundamental Web Development',
        description: 'Dasar-dasar HTML5, CSS Modern (Flexbox/Grid), dan JavaScript ES6+.',
        category: 'Technical',
        program: { name: 'Web Development and UI/UX Design' },
        creatorMentor: { name: 'Mentor Web Team' },
      },
      {
        id: 'comp-2',
        programId: 'prog-web',
        name: 'Kompetensi 2: UI/UX Design & Prototyping',
        description: 'Perancangan antarmuka, wireframing, dan interactive prototyping di Figma.',
        category: 'Technical',
        program: { name: 'Web Development and UI/UX Design' },
        creatorMentor: { name: 'Mentor UI/UX Team' },
      },
      {
        id: 'comp-3',
        programId: 'prog-web',
        name: 'Kompetensi 3: Soft Skills & Professional Development (CCA)',
        description: 'Communication, Critical Thinking, Leadership, dan persiapan karir profesional.',
        category: 'Soft Skills (CCA)',
        program: { name: 'Web Development and UI/UX Design' },
        creatorMentor: { name: 'Mentor Professional Team' },
      },
      {
        id: 'comp-4',
        programId: 'prog-web',
        name: 'Kompetensi 4: Capstone Project Implementation',
        description: 'Proyek akhir kolaboratif lintas disiplin ilmu.',
        category: 'Capstone Project',
        program: { name: 'Web Development and UI/UX Design' },
        creatorMentor: { name: 'Mentor UI/UX Team' },
      },
    ];
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
