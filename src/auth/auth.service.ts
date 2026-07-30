import { Injectable, ForbiddenException, BadRequestException, UnauthorizedException, NotFoundException } from '@nestjs/common';
import { UsersService } from '../users/users.service.js';
import { User, UserStatus } from '../users/entities/user.entity.js';
import * as bcrypt from 'bcryptjs';

export interface GoogleUserPayload {
  email: string;
  name: string;
  avatarUrl: string | null;
  googleId: string;
}

@Injectable()
export class AuthService {
  constructor(private readonly usersService: UsersService) {}

  async validateUser(payload: GoogleUserPayload): Promise<User> {
    const user = await this.usersService.findByEmail(payload.email);

    if (!user) {
      throw new ForbiddenException('Akun kamu belum terdaftar. Hubungi Admin.');
    }

    // First time login: bind Google ID and activate account
    if (!user.googleId) {
      return this.usersService.activateOnFirstLogin(
        user,
        payload.googleId,
        payload.avatarUrl,
      );
    }

    // Subsequent logins: update last login timestamp
    await this.usersService.updateLastLogin(user);
    return user;
  }

  async checkEmailStatus(email: string) {
    const user = await this.usersService.findByEmailWithPassword(email);
    if (!user) {
      throw new ForbiddenException('Akun email tidak ditemukan dalam sistem. Hubungi Admin.');
    }

    const hasPassword = !!user.password;
    const isGoogleBound = !!user.googleId;

    return {
      registered: true,
      hasPassword,
      isGoogleBound,
      name: user.name,
      email: user.email,
    };
  }

  async validateLocalUser(email: string, pass: string): Promise<User> {
    const user = await this.usersService.findByEmailWithPassword(email);
    if (!user) {
      throw new UnauthorizedException('Email atau password salah.');
    }

    if (!user.password) {
      throw new BadRequestException('Password belum dibuat. Silakan buat password pertama Anda terlebih dahulu.');
    }

    const isMatch = await bcrypt.compare(pass, user.password);
    if (!isMatch) {
      throw new UnauthorizedException('Email atau password salah.');
    }

    if (user.status === UserStatus.INVITED) {
      user.status = UserStatus.ACTIVE;
    }
    await this.usersService.updateLastLogin(user);
    return user;
  }

  async setupInitialPassword(email: string, newPass: string): Promise<User> {
    if (!newPass || newPass.length < 6) {
      throw new BadRequestException('Password minimal 6 karakter.');
    }

    const user = await this.usersService.findByEmailWithPassword(email);
    if (!user) {
      throw new ForbiddenException('Email belum terdaftar dalam sistem LMS. Hubungi Admin.');
    }

    const hashedPassword = await bcrypt.hash(newPass, 10);
    user.password = hashedPassword;
    if (user.status === UserStatus.INVITED) {
      user.status = UserStatus.ACTIVE;
    }
    user.isPasswordChanged = true;
    return this.usersService.saveUser(user);
  }

  async changePassword(userId: string, currentPass: string, newPass: string): Promise<User> {
    if (!newPass || newPass.length < 6) {
      throw new BadRequestException('Password baru minimal 6 karakter.');
    }

    const user = await this.usersService.findById(userId);
    if (!user) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    const fullUser = await this.usersService.findByEmailWithPassword(user.email);
    if (!fullUser) {
      throw new NotFoundException('User tidak ditemukan.');
    }

    // Verify current password if user has a password set
    if (fullUser.password) {
      const isMatch = await bcrypt.compare(currentPass || '', fullUser.password);
      if (!isMatch) {
        throw new BadRequestException('Password lama / default salah.');
      }
    }

    const hashedPassword = await bcrypt.hash(newPass, 10);
    fullUser.password = hashedPassword;
    fullUser.isPasswordChanged = true;

    return this.usersService.saveUser(fullUser);
  }
}
