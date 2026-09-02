import { Injectable, Logger } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { S3Client, PutObjectCommand } from '@aws-sdk/client-s3';
import * as crypto from 'crypto';

@Injectable()
export class StorageService {
  private readonly logger = new Logger(StorageService.name);
  private s3Client: S3Client | null = null;

  constructor(private configService: ConfigService) {}

  private getS3Client(): {
    client: S3Client | null;
    bucketName: string;
    publicDomain: string;
  } {
    const accountId = this.configService.get<string>('R2_ACCOUNT_ID', '');
    const accessKeyId = this.configService.get<string>(
      'R2_ACCESS_KEY_ID',
      process.env.R2_ACCESS_KEY_ID || '',
    );
    const secretAccessKey = this.configService.get<string>(
      'R2_SECRET_ACCESS_KEY',
      process.env.R2_SECRET_ACCESS_KEY || '',
    );
    const bucketName = this.configService.get<string>(
      'R2_BUCKET_NAME',
      'lms-v2',
    );
    const publicDomain = this.configService
      .get<string>('R2_PUBLIC_DOMAIN', 'https://media.lms-v2.my.id')
      .replace(/\/$/, '');

    if (!this.s3Client && accessKeyId && secretAccessKey) {
      this.s3Client = new S3Client({
        region: 'auto',
        endpoint: `https://${accountId}.r2.cloudflarestorage.com`,
        credentials: {
          accessKeyId,
          secretAccessKey,
        },
      });
      this.logger.log(
        `Initialized Cloudflare R2 S3 Client for bucket "${bucketName}"`,
      );
    }

    return { client: this.s3Client, bucketName, publicDomain };
  }

  /**
   * Uploads a base64 encoded file string to Cloudflare R2 Storage and returns its public URL.
   */
  async uploadBase64(
    base64Str: string,
    folder = 'permissions',
  ): Promise<string> {
    // If it's already an HTTP/HTTPS URL, return as is
    if (base64Str.startsWith('http://') || base64Str.startsWith('https://')) {
      return base64Str;
    }

    const { client, bucketName, publicDomain } = this.getS3Client();

    if (!client) {
      this.logger.warn(
        'R2 S3 Client not configured (missing R2_ACCESS_KEY_ID or R2_SECRET_ACCESS_KEY). Falling back to direct data handling.',
      );
      return base64Str;
    }

    try {
      // Parse Base64 Header & Data
      const matches = base64Str.match(/^data:(.+?);base64,(.+)$/);
      let mimeType = 'application/octet-stream';
      let buffer: Buffer;

      if (matches && matches.length === 3) {
        mimeType = matches[1];
        buffer = Buffer.from(matches[2], 'base64');
      } else {
        buffer = Buffer.from(base64Str, 'base64');
      }

      // Determine extension from mimeType
      const ext = this.getExtensionFromMimeType(mimeType);
      const filename = `${folder}/${Date.now()}-${crypto.randomBytes(6).toString('hex')}.${ext}`;

      const command = new PutObjectCommand({
        Bucket: bucketName,
        Key: filename,
        Body: buffer,
        ContentType: mimeType,
      });

      await client.send(command);

      const publicUrl = `${publicDomain}/${filename}`;
      this.logger.log(
        `Successfully uploaded file to Cloudflare R2: ${publicUrl}`,
      );
      return publicUrl;
    } catch (error) {
      this.logger.error(
        `Failed to upload file to Cloudflare R2: ${error.message}`,
        error.stack,
      );
      // Fallback to original input if upload fails
      return base64Str;
    }
  }

  /**
   * Upload multiple base64 strings
   */
  async uploadMultipleBase64(
    base64Array: string[],
    folder = 'permissions',
  ): Promise<string[]> {
    if (!base64Array || base64Array.length === 0) return [];
    return Promise.all(
      base64Array.map((str) => this.uploadBase64(str, folder)),
    );
  }

  /**
   * Fetches an external image URL (e.g. Google profile avatar) and uploads it to Cloudflare R2 Storage.
   */
  async uploadUrlToR2(imageUrl: string, folder = 'avatars'): Promise<string> {
    if (!imageUrl) return imageUrl;

    // If it's already on our R2 public domain, return as is
    const { publicDomain, client, bucketName } = this.getS3Client();
    if (imageUrl.startsWith(publicDomain)) {
      return imageUrl;
    }

    if (!client) {
      this.logger.warn(
        'R2 S3 Client not configured. Returning original avatar URL.',
      );
      return imageUrl;
    }

    try {
      const response = await fetch(imageUrl);
      if (!response.ok) {
        this.logger.warn(
          `Failed to fetch remote image from ${imageUrl}: ${response.statusText}`,
        );
        return imageUrl;
      }

      const contentType = response.headers.get('content-type') || 'image/jpeg';
      const arrayBuffer = await response.arrayBuffer();
      const buffer = Buffer.from(arrayBuffer);

      const ext = this.getExtensionFromMimeType(contentType);
      const filename = `${folder}/avatar-${Date.now()}-${crypto.randomBytes(4).toString('hex')}.${ext}`;

      const command = new PutObjectCommand({
        Bucket: bucketName,
        Key: filename,
        Body: buffer,
        ContentType: contentType,
      });

      await client.send(command);

      const publicUrl = `${publicDomain}/${filename}`;
      this.logger.log(
        `Successfully synced remote avatar to Cloudflare R2: ${publicUrl}`,
      );
      return publicUrl;
    } catch (error) {
      this.logger.error(
        `Failed to upload remote avatar URL to R2: ${error.message}`,
      );
      return imageUrl;
    }
  }

  private getExtensionFromMimeType(mimeType: string): string {
    const map: Record<string, string> = {
      'image/jpeg': 'jpg',
      'image/png': 'png',
      'image/webp': 'webp',
      'image/gif': 'gif',
      'application/pdf': 'pdf',
      'text/plain': 'txt',
    };
    return map[mimeType] || 'jpg';
  }
}
