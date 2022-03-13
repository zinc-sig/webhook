import { moveSync, readFileSync, outputFileSync, readdirSync, lstatSync, mkdirSync } from "fs-extra";
import { exec } from 'child_process';
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
      // case 'rar':
      //   extractRAR(submission);
      //   break;
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
    const sourcePath = paths.length===1 && lstatSync(`${temporaryResolvePath}/${paths[0]}`).isDirectory()?`${temporaryResolvePath}/${paths[0]}`:temporaryResolvePath;
    moveSync(sourcePath, `${mountPath}/extracted/${submission.id}`);
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
    mkdirSync(temporaryResolvePath);
    await new Promise<void>((resolve, reject) => {
      exec(`unzip ${file} -d ${temporaryResolvePath}`, (error, _, stderr) =>{
        if(error||stderr){
          console.error(`exec error: ${error||stderr}`)
          reject(error||stderr)
        }
        resolve()
      })
    });
    const files = readdirSync(temporaryResolvePath);
    if(files.length >= 1){
      const sourcePath = files.length===1&&lstatSync(`${temporaryResolvePath}/${files[0]}`).isDirectory()?`${temporaryResolvePath}/${files[0]}`:temporaryResolvePath;
      moveSync(sourcePath, extractToPath)
    } else {
      throw new Error("empty directory")
    }
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
