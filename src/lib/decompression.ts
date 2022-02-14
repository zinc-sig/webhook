import { moveSync, readFileSync, outputFileSync, readdirSync, lstatSync, existsSync, createWriteStream } from "fs-extra";
import yauzl from "yauzl"
import mkdirp from "mkdirp";
import { createExtractorFromData } from "node-unrar-js";
import httpClient from "../utils/http";
import { GET_GRADING_POLICY, UPDATE_DECOMPRESSION_RESULT_FOR_SUBMISSION } from "../utils/queries";

const mountPath = process.env.NODE_ENV==='production'?process.env.SHARED_MOUNT_PATH:'/home/system/workspace'

export async function decompressSubmission(submission: any) {
  try {
    console.log(`[!] Begins deflation process for submission archive #${submission.id} for file: ${submission.upload_name}`);
    const fileExtension = submission.upload_name.slice(submission.upload_name.lastIndexOf('.')+1, submission.upload_name.length);
    switch(fileExtension) {
      case 'zip':
        await extractZip(submission);
        break;
      case 'rar':
        extractRAR(submission);
        break;
      default:
        throw new Error('Unsupported archive format')
    }
    await updateExtractedSubmissionEntry(submission.id, `extracted/${submission.id}`);
  } catch (error) {
    await updateExtractedSubmissionEntry(submission.id, null, error.message);
    console.error(`[✗] Submission: ${submission.id}`,error.message);
    throw error
  }
}
function extractRAR(submission: any) {
  try {
    const rawFileBuffer = Uint8Array.from(readFileSync(`${mountPath}/${submission.stored_name}`)).buffer;
    const extractor = createExtractorFromData(rawFileBuffer);
    const contents = extractor.extractAll();
    const [_, { files }] = contents
    const temporaryResolvePath = `/tmp/${submission.id}`;
    for(const file of files) {
      const { fileHeader, extract } = file;
      if(!fileHeader.flags.directory) {
        const [_, buffer] = extract
        outputFileSync(`${temporaryResolvePath}/${fileHeader.name}`, buffer);
      }
    }
    const paths = readdirSync(temporaryResolvePath);
    moveSync(paths.length===1 && lstatSync(`${temporaryResolvePath}/${paths[0]}`).isDirectory()?`${temporaryResolvePath}/${paths[0]}`:temporaryResolvePath, `${mountPath}/extracted/${submission.id}`);
  } catch (error) {
    console.error(`[✗] Error occurred when deflating RAR archive; Reason: ${error.message}`);
    throw error
  }
}

async function extractZip(submission: any) {
  try {
    const file = `${mountPath}/${submission.stored_name}`;
    const extractToPath = `${mountPath}/extracted/${submission.id}`
    const temporaryResolvePath = `/tmp/${submission.id}`;
    await new Promise<void>((resolve, reject) => {
      yauzl.open(file, { lazyEntries: true }, async function(err, zipFile) {
        if (err) reject(err.message);
        await mkdirp(temporaryResolvePath);
        zipFile.readEntry();
        zipFile.once('end', function() {
          zipFile.close();
          const paths = readdirSync(temporaryResolvePath);
          moveSync(paths.length===1 && lstatSync(`${temporaryResolvePath}/${paths[0]}`).isDirectory()?`${temporaryResolvePath}/${paths[0]}`:temporaryResolvePath, extractToPath);
          resolve();
        })
        zipFile.on('entry', async function(entry) {
          if (/__MACOSX/.test(entry.fileName)||/.DS_Store/.test(entry.fileName)) {
            zipFile.readEntry();
          }
          else if (entry.fileName.includes('/')) {
            if(entry.fileName.indexOf('/')===entry.fileName.length-1) {
              const folderName = entry.fileName.slice(0, entry.fileName.lastIndexOf('/'));
              if(!existsSync(`${temporaryResolvePath}/${folderName}`)) {
                await mkdirp(`${temporaryResolvePath}/${folderName}`);
              }
              zipFile.readEntry();
            } else {
              const folderName = entry.fileName.slice(0, entry.fileName.lastIndexOf('/'));
              if(!existsSync(`${temporaryResolvePath}/${folderName}`)) {
                await mkdirp(`${temporaryResolvePath}/${folderName}`);
              }
              zipFile.openReadStream(entry, function(err, readStream) {
                if (err) reject(err.message);
                readStream.on("end", function() {
                  zipFile.readEntry();
                });
                readStream.pipe(
                  createWriteStream(`${temporaryResolvePath}/${entry.fileName}`)
                )
              });
            }
          } else {
            zipFile.openReadStream(entry, function(err, readStream) {
              if (err) reject(err.message);
              readStream.on("end", function() {
                zipFile.readEntry();
              });
              readStream.pipe(
                createWriteStream(`${temporaryResolvePath}/${entry.fileName}`)
              )
            })
          }
        })
      });
    })
  } catch (error) {
    console.error(`[✗] Error occurred when deflating ZIP archive; Reason: ${error.message}`);
    throw error
  }
}

async function updateExtractedSubmissionEntry(id: number, extractedPath?: string, failReason?: string) {
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: UPDATE_DECOMPRESSION_RESULT_FOR_SUBMISSION,
        variables: {
          id,
          extractedPath,
          failReason
        }
      }
    });
  } catch (error) {
    console.error(`[✗] Error occurred when updating submission entry for submission #${id}; Reason: ${error.message}`);
    throw error;
  }
}

export async function getGradingPolicy(assignmentConfigId: number, userId: number) {
  try {
    const { data: { data }} = await httpClient.request({
      url: '/graphql',
      data: {
        query: GET_GRADING_POLICY,
        variables: {
          id: assignmentConfigId,
          userId
        }
      }
    });
    const { gradeImmediately, assignment: { course: { users }} } = data.assignmentConfig;
    const [user] = users;
    return {
      gradeImmediately,
      isTest: user.permission > 1
    }
  } catch (error) {
    console.error(error)
  }
}
