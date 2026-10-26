import {describe,it,expect} from 'vitest';
import {trafficSample,bytes} from '../src/telemetry';
import type {Metrics} from '../src/types';
const m=(at:number,down:number|null,up:number|null):Metrics=>({observed_at:new Date(at).toISOString(),download_total:down,upload_total:up,connections:0,memory_bytes:0});
describe('traffic observations',()=>{
 it('uses observed time and counter differences, not polling frequency',()=>{expect(trafficSample(m(1000,100,50),m(5000,900,250))).toEqual({at:5000,down:200,up:50})});
 it('does not draw fictitious traffic on startup, reset, missing fields or long gaps',()=>{expect(trafficSample(undefined,m(5000,0,0))).toBeNull();expect(trafficSample(m(1000,100,50),m(5000,0,0))).toBeNull();expect(trafficSample(m(1000,null,0),m(5000,0,0))).toBeNull();expect(trafficSample(m(1000,0,0),m(60000,1000,1000))).toBeNull();expect(bytes(null)).toBe('—');expect(bytes(0)).toBe('0 B')});
});
