import { PassportStrategy } from '@nestjs/passport';
import { Strategy, VerifyCallback } from 'passport-google-oauth20';
import { Injectable, Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';

@Injectable()
export class GoogleStrategy extends PassportStrategy(Strategy, 'google') {
  private readonly logger = new Logger(GoogleStrategy.name);

  constructor(configService: ConfigService) {
    const clientID = configService.get<string>('GOOGLE_CLIENT_ID');
    const clientSecret = configService.get<string>('GOOGLE_CLIENT_SECRET');
    const backendUrl = (
      configService.get<string>('BACKEND_URL') || 'http://localhost:7000'
    ).replace(/\/$/, '');
    const callbackURL = configService.get<string>(
      'GOOGLE_CALLBACK_URL',
      `${backendUrl}/auth/google/callback`,
    );

    if (!clientID || !clientSecret) {
      // Use placeholder values so the server can start without Google credentials.
      // Actual login attempts will fail with a clear error at the controller level.
      super({
        clientID: 'not-configured',
        clientSecret: 'not-configured',
        callbackURL,
        scope: ['email', 'profile'],
      });
      this.logger.warn(
        '⚠ GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET is not set. ' +
          'Google OAuth login will not work until credentials are configured in .env',
      );
    } else {
      super({
        clientID,
        clientSecret,
        callbackURL,
        scope: ['email', 'profile'],
      });
      this.logger.log('Google OAuth strategy initialized successfully.');
    }
  }

  async validate(
    accessToken: string,
    refreshToken: string,
    profile: any,
    done: VerifyCallback,
  ) {
    const { emails, displayName, photos, id } = profile;
    const googleUser = {
      email: emails[0].value,
      name: displayName,
      avatarUrl: photos?.[0]?.value ?? null,
      googleId: id,
    };
    done(null, googleUser);
  }
}
