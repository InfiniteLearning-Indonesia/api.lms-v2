import { PassportSerializer } from '@nestjs/passport';
import { Injectable } from '@nestjs/common';
import { UsersService } from '../../users/users.service.js';
import { User, UserStatus } from '../../users/entities/user.entity.js';

@Injectable()
export class SessionSerializer extends PassportSerializer {
  constructor(private readonly usersService: UsersService) {
    super();
  }

  serializeUser(user: any, done: (err: Error | null, key: string) => void) {
    const key = user.id || user.email;
    if (!key) {
      return done(new Error('Failed to find serializable key in user object'), '');
    }
    done(null, key);
  }

  async deserializeUser(
    key: string,
    done: (err: Error | null, user: User | null) => void,
  ) {
    try {
      let user: User | null = null;
      if (key.includes('@')) {
        user = await this.usersService.findByEmail(key);
      } else {
        user = await this.usersService.findById(key);
      }
      if (user && user.status === UserStatus.SUSPENDED) {
        return done(new Error('Akun kamu telah di-suspend oleh Admin. Akses ditolak.'), null);
      }
      done(null, user);
    } catch (err: any) {
      done(err, null);
    }
  }
}
