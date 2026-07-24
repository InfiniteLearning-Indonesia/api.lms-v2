import { Injectable, ForbiddenException } from '@nestjs/common';
import { UsersService } from '../users/users.service.js';
import { User, UserStatus } from '../users/entities/user.entity.js';

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
}
