import { Injectable, Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as nodemailer from 'nodemailer';

@Injectable()
export class MailService {
  private readonly logger = new Logger(MailService.name);
  private transporter: nodemailer.Transporter | null = null;

  constructor(private readonly configService: ConfigService) {
    const host = this.configService.get<string>('SMTP_HOST');
    const port = this.configService.get<number>('SMTP_PORT', 587);
    const user = this.configService.get<string>('SMTP_USER');
    const pass = this.configService.get<string>('SMTP_PASS');

    const enableBlast = this.configService.get<string>('ENABLE_EMAIL_BLAST') === 'true';

    if (host && user && pass && enableBlast) {
      this.transporter = nodemailer.createTransport({
        host,
        port,
        secure: port === 465, // true for 465, false for other ports
        auth: {
          user,
          pass,
        },
      });
      this.logger.log('SMTP mail transporter initialized successfully (Real Email Sending Enabled).');
    } else {
      this.transporter = null;
      this.logger.warn(
        '⚠ SMTP is disabled or ENABLE_EMAIL_BLAST is not set to true. ' +
          'Email invitations will be printed to console log (mocked) for safety.',
      );
    }
  }

  async sendInvitation(
    email: string,
    name: string,
    role: string,
  ): Promise<boolean> {
    const frontendUrl = this.configService.get<string>(
      'FRONTEND_URL',
      'http://localhost:3000',
    );
    const loginUrl = `${frontendUrl}/login`;
    const from = this.configService.get<string>(
      'SMTP_FROM',
      '"Infinite Learning LMS" <no-reply@infinitelearning.id>',
    );

    const roleLabels: Record<string, string> = {
      admin: 'Administrator',
      mentor: 'Mentor Kelas',
      student: 'Siswa LMS',
    };
    const roleLabel = roleLabels[role] || role;

    const htmlContent = `
      <div style="font-family: sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; border: 1px solid #e5e5e5; border-radius: 8px;">
        <h2 style="color: #8A3DFF; margin-bottom: 20px;">Undangan Ruang Kelas Infinite Learning</h2>
        <p>Halo <strong>${name}</strong>,</p>
        <p>Anda telah diundang untuk bergabung di LMS Infinite Learning dengan peran sebagai <strong>${roleLabel}</strong>.</p>
        <p>Silakan masuk menggunakan Akun Google Anda yang terdaftar dengan email: <strong>${email}</strong>.</p>
        <div style="margin: 30px 0; text-align: center;">
          <a href="${loginUrl}" style="background-color: #8A3DFF; color: white; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold; display: inline-block;">
            Masuk ke LMS
          </a>
        </div>
        <p style="font-size: 12px; color: #ff0000; font-weight: bold; margin-top: 20px;">
          PENTING (Gunakan Akun Google Pribadi):
        </p>
        <p style="font-size: 12px; color: #666; line-height: 1.5;">
          Sistem login kami menggunakan Google OAuth. Karena alamat email ini (<strong>${email}</strong>) bukan merupakan Akun Google (Gmail), Anda tidak akan bisa masuk. Silakan hubungi atau ajukan pendaftaran ulang ke mentor Anda untuk mendaftarkan alamat email Google (Gmail) pribadi Anda yang aktif.
        </p>
        <hr style="border: 0; border-top: 1px solid #eee; margin-top: 30px;" />
        <p style="font-size: 11px; color: #999; text-align: center;">
          &copy; ${new Date().getFullYear()} Infinite Learning. Hak cipta dilindungi.
        </p>
      </div>
    `;

    if (this.transporter) {
      try {
        await this.transporter.sendMail({
          from,
          to: email,
          subject: 'Undangan Bergabung Kelas LMS Infinite Learning',
          html: htmlContent,
        });
        this.logger.log(`Invitation email sent successfully to ${email}`);
        return true;
      } catch (err: any) {
        this.logger.error(
          `Failed to send invitation email to ${email}: ${err.message}`,
        );
        return false;
      }
    } else {
      this.logger.log(`
=========================================
[MOCK EMAIL SENT]
To: ${email}
Subject: Undangan Bergabung Kelas LMS Infinite Learning
Login Link: ${loginUrl}
Role: ${roleLabel}
=========================================
      `);
      return true;
    }
  }
}
